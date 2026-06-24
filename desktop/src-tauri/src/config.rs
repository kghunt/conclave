use serde::{Deserialize, Serialize};
use std::path::PathBuf;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Config {
    pub instance_url: String,
    pub shortcuts: ShortcutConfig,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ShortcutConfig {
    pub mute_key: String,
    pub window_key: String,
    /// "toggle" | "push_to_mute" | "push_to_talk"
    pub mute_mode: String,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            instance_url: String::new(),
            shortcuts: ShortcutConfig::default(),
        }
    }
}

impl Default for ShortcutConfig {
    fn default() -> Self {
        Self {
            mute_key: "ctrl+shift+m".into(),
            window_key: "ctrl+shift+d".into(),
            mute_mode: "toggle".into(),
        }
    }
}

fn config_path() -> PathBuf {
    directories::ProjectDirs::from("com", "conclave", "desktop")
        .expect("could not determine config directory")
        .config_dir()
        .join("config.toml")
}

impl Config {
    pub fn load() -> Self {
        let path = config_path();
        std::fs::read_to_string(&path)
            .ok()
            .and_then(|s| toml::from_str(&s).ok())
            .unwrap_or_default()
    }

    pub fn save(&self) {
        let path = config_path();
        if let Some(dir) = path.parent() {
            let _ = std::fs::create_dir_all(dir);
        }
        if let Ok(s) = toml::to_string(self) {
            let _ = std::fs::write(path, s);
        }
    }

    pub fn is_configured(&self) -> bool {
        !self.instance_url.is_empty()
    }
}
