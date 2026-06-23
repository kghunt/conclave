mod api;
mod config;
mod detector;

use std::sync::{Arc, Mutex};
use std::time::Duration;
use tauri::{
    menu::{Menu, MenuItem},
    tray::{MouseButton, TrayIconBuilder, TrayIconEvent},
    AppHandle, Manager,
};
use tauri_plugin_deep_link::DeepLinkExt;

struct AppState {
    config: Arc<Mutex<config::Config>>,
}

#[tauri::command]
fn get_config(state: tauri::State<AppState>) -> serde_json::Value {
    let cfg = state.config.lock().unwrap();
    serde_json::json!({
        "instance_url": cfg.instance_url,
        "is_configured": cfg.is_configured(),
    })
}

#[tauri::command]
fn apply_config(instance_url: String, token: String, state: tauri::State<AppState>) {
    let mut cfg = state.config.lock().unwrap();
    cfg.instance_url = instance_url;
    cfg.token = token;
    cfg.save();
}

// Shared URL handler — called from both on_open_url and get_current().
fn handle_connect_url(url_str: &str, cfg_ref: &Arc<Mutex<config::Config>>, app: &AppHandle) {
    let query = if let Some(q) = url_str.strip_prefix("conclave://connect?") {
        q
    } else {
        return;
    };

    let mut instance = String::new();
    let mut token = String::new();
    for pair in query.split('&') {
        if let Some((k, v)) = pair.split_once('=') {
            match k {
                "instance" => instance = url_decode(v),
                "token"    => token    = url_decode(v),
                _ => {}
            }
        }
    }

    if instance.is_empty() || token.is_empty() {
        return;
    }

    {
        let mut cfg = cfg_ref.lock().unwrap();
        cfg.instance_url = instance;
        cfg.token = token;
        cfg.save();
    }

    if let Some(win) = app.get_webview_window("main") {
        win.show().ok();
        win.set_focus().ok();
    }
}

pub fn run() {
    env_logger::init();

    let cfg = Arc::new(Mutex::new(config::Config::load()));
    let cfg_bg = cfg.clone();

    tauri::Builder::default()
        .plugin(tauri_plugin_single_instance::init(|_app, _args, _cwd| {}))
        .plugin(tauri_plugin_deep_link::init())
        .manage(AppState { config: cfg })
        .setup(move |app| {
            // Hide the main window on startup — we live in the tray.
            if let Some(win) = app.get_webview_window("main") {
                win.hide().ok();
            }

            // Tray icon.
            let quit = MenuItem::with_id(app, "quit", "Quit Conclave Presence", true, None::<&str>)?;
            let open = MenuItem::with_id(app, "open", "Open Settings", true, None::<&str>)?;
            let menu = Menu::with_items(app, &[&open, &quit])?;

            let _tray = TrayIconBuilder::new()
                .icon(app.default_window_icon().unwrap().clone())
                .menu(&menu)
                .on_menu_event(|app, event| match event.id.as_ref() {
                    "quit" => app.exit(0),
                    "open" => {
                        if let Some(win) = app.get_webview_window("main") {
                            win.show().ok();
                            win.set_focus().ok();
                        }
                    }
                    _ => {}
                })
                .on_tray_icon_event(|tray, event| {
                    if let TrayIconEvent::Click { button: MouseButton::Left, .. } = event {
                        let app = tray.app_handle();
                        if let Some(win) = app.get_webview_window("main") {
                            win.show().ok();
                            win.set_focus().ok();
                        }
                    }
                })
                .build(app)?;

            // Deep link handling.
            let state: tauri::State<AppState> = app.state();
            let cfg_link  = state.config.clone();
            let cfg_link2 = state.config.clone();
            let handle    = app.handle().clone();
            let handle2   = app.handle().clone();

            // Hot path: app already running, URL forwarded by single-instance plugin.
            app.deep_link().on_open_url(move |event| {
                for url in event.urls() {
                    handle_connect_url(url.as_str(), &cfg_link, &handle);
                }
            });

            // Cold start path: URL was passed as a launch argument.
            // get_current() returns the URL that triggered this process.
            if let Ok(Some(urls)) = app.deep_link().get_current() {
                for url in urls {
                    handle_connect_url(url.as_str(), &cfg_link2, &handle2);
                }
            }

            // Background heartbeat loop.
            let games = detector::load_games();
            tauri::async_runtime::spawn(async move {
                let mut last_game: Option<String> = None;
                loop {
                    tokio::time::sleep(Duration::from_secs(30)).await;
                    let cfg = cfg_bg.lock().unwrap().clone();
                    if !cfg.is_configured() {
                        continue;
                    }
                    let game = detector::detect_running_game(&games);
                    if game != last_game {
                        last_game = game.clone();
                    }
                    if let Err(e) = api::heartbeat(&cfg, game.as_deref()).await {
                        log::warn!("heartbeat failed: {e}");
                    }
                }
            });

            Ok(())
        })
        .invoke_handler(tauri::generate_handler![get_config, apply_config])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

fn url_decode(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    let mut bytes = s.bytes().peekable();
    while let Some(b) = bytes.next() {
        if b == b'%' {
            let hi = bytes.next().unwrap_or(b'0');
            let lo = bytes.next().unwrap_or(b'0');
            if let Ok(c) = u8::from_str_radix(
                &format!("{}{}", hi as char, lo as char), 16,
            ) {
                out.push(c as char);
            }
        } else if b == b'+' {
            out.push(' ');
        } else {
            out.push(b as char);
        }
    }
    out
}
