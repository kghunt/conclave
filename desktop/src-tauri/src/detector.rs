use serde::{Deserialize, Serialize};
use sysinfo::System;

/// A recognized game entry from games.json.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GameEntry {
    pub name: String,
    pub processes: Vec<String>,
}

pub fn load_games() -> Vec<GameEntry> {
    let json = include_str!("../games.json");
    serde_json::from_str(json).unwrap_or_default()
}

/// Returns the first recognized game name that is currently running,
/// or None if no recognized game process is found.
pub fn detect_running_game(games: &[GameEntry]) -> Option<String> {
    let mut sys = System::new_all();
    sys.refresh_processes(sysinfo::ProcessesToUpdate::All, true);

    let running: std::collections::HashSet<String> = sys
        .processes()
        .values()
        .map(|p| p.name().to_string_lossy().to_lowercase())
        .collect();

    for game in games {
        for proc in &game.processes {
            if running.contains(&proc.to_lowercase()) {
                return Some(game.name.clone());
            }
        }
    }
    None
}
