use serde::{Deserialize, Serialize};
use std::path::PathBuf;

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct Config {
    pub instance_url: String,
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
