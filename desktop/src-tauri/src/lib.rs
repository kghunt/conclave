mod api;
mod config;
mod detector;

use std::sync::{Arc, Mutex};
use std::time::Duration;
use tauri::{
    menu::{Menu, MenuItem},
    tray::{MouseButton, TrayIconBuilder, TrayIconEvent},
    Manager,
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

            // Tray icon setup.
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

            // Handle deep links: conclave://connect?instance=...&token=...
            let state: tauri::State<AppState> = app.state();
            let cfg_link = state.config.clone();
            let app_handle = app.handle().clone();

            app.deep_link().on_open_url(move |event| {
                for url in event.urls() {
                    let url_str = url.as_str();
                    if let Some(query) = url_str.strip_prefix("conclave://connect?") {
                        let mut instance = String::new();
                        let mut token = String::new();
                        for pair in query.split('&') {
                            if let Some((k, v)) = pair.split_once('=') {
                                let decoded = url_decode(v);
                                match k {
                                    "instance" => instance = decoded,
                                    "token" => token = decoded,
                                    _ => {}
                                }
                            }
                        }
                        if !instance.is_empty() && !token.is_empty() {
                            let mut cfg = cfg_link.lock().unwrap();
                            cfg.instance_url = instance;
                            cfg.token = token;
                            cfg.save();
                            drop(cfg);
                            if let Some(win) = app_handle.get_webview_window("main") {
                                win.show().ok();
                                win.set_focus().ok();
                            }
                        }
                    }
                }
            });

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
    let mut chars = s.bytes().peekable();
    while let Some(b) = chars.next() {
        if b == b'%' {
            let hi = chars.next().unwrap_or(b'0');
            let lo = chars.next().unwrap_or(b'0');
            if let Ok(c) = u8::from_str_radix(&format!("{}{}", hi as char, lo as char), 16) {
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
