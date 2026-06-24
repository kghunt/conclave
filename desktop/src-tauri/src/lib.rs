mod config;
mod detector;

use std::sync::{Arc, Mutex};
use std::time::Duration;
use tauri::{
    menu::{Menu, MenuItem},
    tray::{MouseButton, TrayIconBuilder, TrayIconEvent},
    Manager, WebviewUrl, WebviewWindowBuilder,
};
use tauri_plugin_global_shortcut::{Code, GlobalShortcutExt, Modifiers, Shortcut};

struct AppState {
    config: Arc<Mutex<config::Config>>,
    games: Arc<Mutex<Vec<detector::GameEntry>>>,
}

fn games_path() -> std::path::PathBuf {
    directories::ProjectDirs::from("com", "conclave", "desktop")
        .expect("could not determine config directory")
        .config_dir()
        .join("games.json")
}

fn load_games_from_disk() -> Vec<detector::GameEntry> {
    let path = games_path();
    if path.exists() {
        if let Ok(s) = std::fs::read_to_string(&path) {
            if let Ok(g) = serde_json::from_str(&s) {
                return g;
            }
        }
    }
    detector::load_games() // fall back to embedded default
}

fn save_games_to_disk(games: &[detector::GameEntry]) {
    let path = games_path();
    if let Some(dir) = path.parent() { let _ = std::fs::create_dir_all(dir); }
    if let Ok(s) = serde_json::to_string_pretty(games) {
        let _ = std::fs::write(path, s);
    }
}

// Called by the setup page after the user enters their instance URL.
#[tauri::command]
fn configure(url: String, state: tauri::State<AppState>, app: tauri::AppHandle) -> Result<(), String> {
    {
        let mut cfg = state.config.lock().unwrap();
        cfg.instance_url = url.clone();
        cfg.save();
    }
    navigate_to_instance(&app, &url)
}

#[tauri::command]
fn get_instance_url(state: tauri::State<AppState>) -> String {
    state.config.lock().unwrap().instance_url.clone()
}

#[tauri::command]
fn get_games(state: tauri::State<AppState>) -> Vec<detector::GameEntry> {
    state.games.lock().unwrap().clone()
}

#[tauri::command]
fn save_games(games: Vec<detector::GameEntry>, state: tauri::State<AppState>) {
    let mut lock = state.games.lock().unwrap();
    *lock = games.clone();
    drop(lock);
    save_games_to_disk(&games);
}

fn navigate_to_instance(app: &tauri::AppHandle, url: &str) -> Result<(), String> {
    let parsed = url::Url::parse(url).map_err(|e| e.to_string())?;
    if let Some(win) = app.get_webview_window("main") {
        let _ = win.navigate(parsed);
        win.show().ok();
        win.set_focus().ok();
    }
    Ok(())
}

pub fn run() {
    env_logger::init();

    let cfg = Arc::new(Mutex::new(config::Config::load()));
    let cfg_bg = cfg.clone();
    let games = Arc::new(Mutex::new(load_games_from_disk()));
    let games_bg = games.clone();

    tauri::Builder::default()
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            if let Some(win) = app.get_webview_window("main") {
                win.show().ok();
                win.set_focus().ok();
            }
        }))
        .plugin(tauri_plugin_global_shortcut::Builder::new().build())
        .manage(AppState { config: cfg, games })
        .setup(move |app| {
            // ── Tray ──────────────────────────────────────────────────────────
            let open = MenuItem::with_id(app, "open", "Open Conclave", true, None::<&str>)?;
            let quit = MenuItem::with_id(app, "quit", "Quit", true, None::<&str>)?;
            let menu = Menu::with_items(app, &[&open, &quit])?;

            TrayIconBuilder::new()
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

            // ── Main window ───────────────────────────────────────────────────
            // Starts on the bundled setup page. If already configured, we
            // immediately navigate to the instance URL.
            // initialization_script runs on every navigation so the SvelteKit
            // app always sees window.__TAURI_DESKTOP__ = true.
            let win = WebviewWindowBuilder::new(
                app,
                "main",
                WebviewUrl::App("index.html".into()),
            )
            .title("Conclave")
            .inner_size(1280.0, 800.0)
            .min_inner_size(900.0, 600.0)
            .initialization_script("window.__TAURI_DESKTOP__ = true;")
            .build()?;

            let instance_url = cfg_bg.lock().unwrap().instance_url.clone();
            if !instance_url.is_empty() {
                if let Ok(url) = url::Url::parse(&instance_url) {
                    win.navigate(url);
                }
            }

            win.show()?;

            // Hide to tray on close instead of quitting.
            let app_for_close = app.handle().clone();
            win.on_window_event(move |event| {
                if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                    api.prevent_close();
                    if let Some(w) = app_for_close.get_webview_window("main") {
                        w.hide().ok();
                    }
                }
            });

            // ── Global shortcuts ──────────────────────────────────────────────
            // Ctrl/Cmd + Shift + M  →  mute toggle (dispatched into WebView)
            // Ctrl/Cmd + Shift + D  →  show / hide window
            let mute_shortcut = Shortcut::new(
                Some(Modifiers::CONTROL | Modifiers::SHIFT),
                Code::KeyM,
            );
            let hide_shortcut = Shortcut::new(
                Some(Modifiers::CONTROL | Modifiers::SHIFT),
                Code::KeyD,
            );

            let app_sh = app.handle().clone();
            let mute_copy = mute_shortcut.clone();
            app.global_shortcut().on_shortcuts(
                [mute_shortcut, hide_shortcut],
                move |_app, shortcut, event| {
                    if let tauri_plugin_global_shortcut::ShortcutState::Pressed = event.state() {
                        if shortcut == &mute_copy {
                            if let Some(w) = app_sh.get_webview_window("main") {
                                w.eval("window.dispatchEvent(new CustomEvent('conclave-shortcut',{detail:'mute'}));").ok();
                            }
                        } else {
                            if let Some(w) = app_sh.get_webview_window("main") {
                                if w.is_visible().unwrap_or(false) {
                                    w.hide().ok();
                                } else {
                                    w.show().ok();
                                    w.set_focus().ok();
                                }
                            }
                        }
                    }
                },
            )?;

            // ── Game detection loop ───────────────────────────────────────────
            let app_game = app.handle().clone();
            tauri::async_runtime::spawn(async move {
                let mut last_game: Option<String> = None;
                loop {
                    tokio::time::sleep(Duration::from_secs(30)).await;
                    let current_games = games_bg.lock().unwrap().clone();
                    let game = detector::detect_running_game(&current_games);
                    if game != last_game {
                        last_game = game.clone();
                        if let Some(win) = app_game.get_webview_window("main") {
                            let payload = serde_json::to_string(&game)
                                .unwrap_or_else(|_| "null".into());
                            win.eval(&format!(
                                "window.dispatchEvent(new CustomEvent('conclave-game',{{detail:{payload}}}));"
                            ))
                            .ok();
                        }
                    }
                }
            });

            Ok(())
        })
        .invoke_handler(tauri::generate_handler![configure, get_instance_url, get_games, save_games])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
