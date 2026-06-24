mod config;
mod detector;

use std::sync::{Arc, Mutex};
use std::time::Duration;
use tauri::{
    menu::{Menu, MenuItem},
    tray::{MouseButton, TrayIconBuilder, TrayIconEvent},
    Manager, WebviewUrl, WebviewWindowBuilder,
};
use tauri_plugin_global_shortcut::{Code, GlobalShortcutExt, Modifiers, Shortcut, ShortcutState};

struct AppState {
    config: Arc<Mutex<config::Config>>,
    games: Arc<Mutex<Vec<detector::GameEntry>>>,
}

// ── Shortcut parsing ──────────────────────────────────────────────────────────

fn parse_shortcut(s: &str) -> Option<Shortcut> {
    let mut modifiers = Modifiers::empty();
    let mut key_code: Option<Code> = None;
    for part in s.to_lowercase().split('+') {
        match part.trim() {
            "ctrl" | "control" => modifiers |= Modifiers::CONTROL,
            "shift"             => modifiers |= Modifiers::SHIFT,
            "alt"               => modifiers |= Modifiers::ALT,
            "meta" | "super" | "win" | "cmd" => modifiers |= Modifiers::META,
            k => key_code = str_to_code(k),
        }
    }
    key_code.map(|c| Shortcut::new(if modifiers.is_empty() { None } else { Some(modifiers) }, c))
}

fn str_to_code(k: &str) -> Option<Code> {
    Some(match k {
        "a" => Code::KeyA, "b" => Code::KeyB, "c" => Code::KeyC, "d" => Code::KeyD,
        "e" => Code::KeyE, "f" => Code::KeyF, "g" => Code::KeyG, "h" => Code::KeyH,
        "i" => Code::KeyI, "j" => Code::KeyJ, "k" => Code::KeyK, "l" => Code::KeyL,
        "m" => Code::KeyM, "n" => Code::KeyN, "o" => Code::KeyO, "p" => Code::KeyP,
        "q" => Code::KeyQ, "r" => Code::KeyR, "s" => Code::KeyS, "t" => Code::KeyT,
        "u" => Code::KeyU, "v" => Code::KeyV, "w" => Code::KeyW, "x" => Code::KeyX,
        "y" => Code::KeyY, "z" => Code::KeyZ,
        "0" => Code::Digit0, "1" => Code::Digit1, "2" => Code::Digit2,
        "3" => Code::Digit3, "4" => Code::Digit4, "5" => Code::Digit5,
        "6" => Code::Digit6, "7" => Code::Digit7, "8" => Code::Digit8, "9" => Code::Digit9,
        "f1"  => Code::F1,  "f2"  => Code::F2,  "f3"  => Code::F3,  "f4"  => Code::F4,
        "f5"  => Code::F5,  "f6"  => Code::F6,  "f7"  => Code::F7,  "f8"  => Code::F8,
        "f9"  => Code::F9,  "f10" => Code::F10, "f11" => Code::F11, "f12" => Code::F12,
        "space"      => Code::Space,
        "backquote" | "`" => Code::Backquote,
        "minus" | "-"     => Code::Minus,
        "equal" | "="     => Code::Equal,
        "tab"        => Code::Tab,
        "capslock"   => Code::CapsLock,
        _ => return None,
    })
}

// ── Shortcut registration ─────────────────────────────────────────────────────

fn register_shortcuts(app: &tauri::AppHandle, sc: &config::ShortcutConfig) -> Result<(), String> {
    app.global_shortcut().unregister_all().map_err(|e| e.to_string())?;

    let mute_sc = parse_shortcut(&sc.mute_key)
        .ok_or_else(|| format!("Invalid mute shortcut: {}", sc.mute_key))?;
    let win_sc = parse_shortcut(&sc.window_key)
        .ok_or_else(|| format!("Invalid window shortcut: {}", sc.window_key))?;

    let app_h = app.clone();
    let mute_copy = mute_sc.clone();
    let mode = sc.mute_mode.clone();

    app.global_shortcut()
        .on_shortcuts([mute_sc, win_sc], move |_app, shortcut, event| {
            if shortcut == &mute_copy {
                let js = match (mode.as_str(), event.state()) {
                    ("push_to_talk", ShortcutState::Pressed)  =>
                        "window.dispatchEvent(new CustomEvent('conclave-shortcut',{detail:'ptt-start'}));",
                    ("push_to_talk", ShortcutState::Released) =>
                        "window.dispatchEvent(new CustomEvent('conclave-shortcut',{detail:'ptt-end'}));",
                    ("push_to_mute", ShortcutState::Pressed)  =>
                        "window.dispatchEvent(new CustomEvent('conclave-shortcut',{detail:'ptm-start'}));",
                    ("push_to_mute", ShortcutState::Released) =>
                        "window.dispatchEvent(new CustomEvent('conclave-shortcut',{detail:'ptm-end'}));",
                    (_, ShortcutState::Pressed) =>
                        "window.dispatchEvent(new CustomEvent('conclave-shortcut',{detail:'mute'}));",
                    _ => return, // toggle: ignore release
                };
                if let Some(w) = app_h.get_webview_window("main") { w.eval(js).ok(); }
            } else if let ShortcutState::Pressed = event.state() {
                if let Some(w) = app_h.get_webview_window("main") {
                    if w.is_visible().unwrap_or(false) { w.hide().ok(); }
                    else { w.show().ok(); w.set_focus().ok(); }
                }
            }
        })
        .map_err(|e| e.to_string())
}

// ── Games persistence ─────────────────────────────────────────────────────────

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
            if let Ok(g) = serde_json::from_str(&s) { return g; }
        }
    }
    detector::load_games()
}

fn save_games_to_disk(games: &[detector::GameEntry]) {
    let path = games_path();
    if let Some(dir) = path.parent() { let _ = std::fs::create_dir_all(dir); }
    if let Ok(s) = serde_json::to_string_pretty(games) { let _ = std::fs::write(path, s); }
}

// ── Tauri commands ────────────────────────────────────────────────────────────

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
fn get_shortcuts(state: tauri::State<AppState>) -> config::ShortcutConfig {
    state.config.lock().unwrap().shortcuts.clone()
}

#[tauri::command]
fn save_shortcuts(
    shortcuts: config::ShortcutConfig,
    state: tauri::State<AppState>,
    app: tauri::AppHandle,
) -> Result<(), String> {
    register_shortcuts(&app, &shortcuts)?;
    let mut cfg = state.config.lock().unwrap();
    cfg.shortcuts = shortcuts;
    cfg.save();
    Ok(())
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

// ── Entry point ───────────────────────────────────────────────────────────────

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
                if let Ok(url) = url::Url::parse(&instance_url) { win.navigate(url); }
            }

            win.show()?;

            // On Linux/WebKitGTK, getUserMedia permission requests must be
            // explicitly granted — the WebView does not auto-allow them.
            #[cfg(target_os = "linux")]
            win.with_webview(|wv| {
                use webkit2gtk::{PermissionRequestExt, SettingsExt, WebViewExt};
                let inner = wv.inner();
                inner.connect_permission_request(|_, request| {
                    request.allow();
                    true
                });
                if let Some(settings) = inner.settings() {
                    settings.set_enable_media_stream(true);
                    settings.set_enable_media_capabilities_api(true);
                }
            })?;

            let app_for_close = app.handle().clone();
            win.on_window_event(move |event| {
                if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                    api.prevent_close();
                    if let Some(w) = app_for_close.get_webview_window("main") { w.hide().ok(); }
                }
            });

            // ── Global shortcuts ──────────────────────────────────────────────
            let initial_shortcuts = cfg_bg.lock().unwrap().shortcuts.clone();
            register_shortcuts(app.handle(), &initial_shortcuts)
                .unwrap_or_else(|e| log::warn!("Shortcut registration failed: {e}"));

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
                            let payload = serde_json::to_string(&game).unwrap_or_else(|_| "null".into());
                            win.eval(&format!(
                                "window.dispatchEvent(new CustomEvent('conclave-game',{{detail:{payload}}}));"
                            )).ok();
                        }
                    }
                }
            });

            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            configure, get_instance_url,
            get_shortcuts, save_shortcuts,
            get_games, save_games,
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
