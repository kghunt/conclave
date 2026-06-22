use crate::config::Config;

pub async fn heartbeat(cfg: &Config, game: Option<&str>) -> Result<(), String> {
    let url = format!("{}/api/presence/heartbeat", cfg.instance_url.trim_end_matches('/'));
    let body = serde_json::json!({ "game": game.unwrap_or("") });

    reqwest::Client::new()
        .post(&url)
        .header("Authorization", format!("Bearer {}", cfg.token))
        .json(&body)
        .send()
        .await
        .map_err(|e| e.to_string())?
        .error_for_status()
        .map_err(|e| e.to_string())?;

    Ok(())
}
