use serde::{Deserialize, Serialize};
use std::path::PathBuf;

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct Config {
    pub instance_url: String,
    pub token: String,
}

fn config_path() -> PathBuf {
    let dirs = directories::ProjectDirs::from("com", "conclave", "presence")
        .expect("could not determine config directory");
    dirs.config_dir().join("config.toml")
}

impl Config {
    pub fn load() -> Self {
        let path = config_path();
        if let Ok(raw) = std::fs::read_to_string(&path) {
            toml::from_str(&raw).unwrap_or_default()
        } else {
            Self::default()
        }
    }

    pub fn save(&self) {
        let path = config_path();
        if let Some(parent) = path.parent() {
            let _ = std::fs::create_dir_all(parent);
        }
        let raw = toml::to_string(self).unwrap_or_default();
        let _ = std::fs::write(path, raw);
    }

    pub fn is_configured(&self) -> bool {
        !self.instance_url.is_empty() && !self.token.is_empty()
    }
}
