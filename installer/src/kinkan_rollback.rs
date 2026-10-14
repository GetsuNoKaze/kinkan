//! Panel-requested rollback to the last running image. Image authority lives in a
//! root-owned file outside the panel's mount; a request supplies version names only.
use super::{Asked, Before, Phase, report, roll_back, start, update_dir};
use crate::{DIR, addon, backup, clock, docker, envfile::EnvFile, ops::Install, panelfs, release, setup, signals};
use anyhow::{Context, Result, bail};
use serde::{Deserialize, Serialize};
use std::{fs, path::Path, process::Command};

const RECORD: &str = "kinkan-rollback.json";

#[derive(Serialize, Deserialize)]
struct Candidate {
    current: String,
    version: String,
    image: String,
    available: bool,
    reason: String,
    at: String,
}

pub(super) fn asked(v: &serde_json::Value) -> Asked {
    match (v["from"].as_str(), v["version"].as_str()) {
        (Some(from), Some(version)) if release::valid_version(from) && release::valid_version(version) => {
            Asked::Rollback(from.to_owned(), version.to_owned())
        }
        _ => Asked::Garbage("rollback needs current and previous release versions".into()),
    }
}

fn valid_image(id: &str) -> bool {
    id.strip_prefix("sha256:").is_some_and(|h| h.len() == 64 && h.bytes().all(|b| b.is_ascii_hexdigit()))
}

pub(super) fn schema_unchanged(output: &str, previous: &str) -> bool {
    // Older Mikan does not read the fork's tables. For a previous Kinkan image,
    // require a positive report for both schemas, not merely a missing fork line.
    if previous.contains("-tt.") && !output.lines().any(|line| line.trim().starts_with("Kinkan schema version: ")) {
        return false;
    }
    super::kinkan_schema_change(output, previous).is_some_and(|(a, b)| a == b)
}

pub(super) fn image_id() -> Result<String> {
    // Tags may already point at the newly pulled image. Capture the image of the
    // existing container, which is the one that actually served the previous version.
    let container = docker::compose_run(&["ps", "-aq", "panel"])?;
    let container = String::from_utf8(container.stdout)?.trim().to_owned();
    if container.is_empty() || container.lines().count() != 1 {
        bail!("no unique previous panel container");
    }
    let output = Command::new("docker").args(["inspect", "--format", "{{.Image}}", &container]).output()?;
    let id = String::from_utf8(output.stdout)?.trim().to_owned();
    if !output.status.success() || !valid_image(&id) {
        bail!("the previous Docker image is unavailable");
    }
    Ok(id)
}

fn publish(c: &Candidate) -> Result<()> {
    let dir = update_dir(false, true)?.context("panel update directory is unavailable")?;
    publish_in(&dir, c)
}

fn publish_in(dir: &panelfs::Dir, c: &Candidate) -> Result<()> {
    let view = serde_json::json!({"current":c.current,"version":c.version,"available":c.available,"reason":c.reason,"at":c.at});
    dir.write("rollback.json", view.to_string().as_bytes(), 0o644)
}

pub(super) fn remember(install: &Install, previous: &str, image: Option<String>, unchanged: bool, say: &mut dyn FnMut(&str)) {
    let current = install.version();
    if install.node || (current == previous && unchanged) {
        return;
    }
    let c = candidate(&current, previous, image, unchanged);
    if let Err(e) = panelfs::write_root(&Path::new(DIR).join(RECORD), &serde_json::to_vec(&c).unwrap()).and_then(|_| publish(&c)) {
        say(&format!("Rollback is unavailable: {e:#}"));
    }
}

fn candidate(current: &str, previous: &str, image: Option<String>, unchanged: bool) -> Candidate {
    let image = image.unwrap_or_default();
    let available = unchanged
        && valid_image(&image)
        && release::valid_version(current)
        && release::valid_version(previous)
        && release::newer(current, previous);
    let reason = if !unchanged {
        "schema_changed"
    } else if !valid_image(&image) {
        "image_missing"
    } else if !available {
        "version_unsupported"
    } else {
        ""
    };
    Candidate { current: current.to_owned(), version: previous.to_owned(), image, available, reason: reason.into(), at: clock::rfc3339() }
}

fn validate(c: &Candidate, current: &str, from: &str, version: &str) -> Result<()> {
    if !c.available
        || c.current != current
        || current != from
        || c.version != version
        || !release::valid_version(version)
        || !release::newer(current, version)
        || !valid_image(&c.image)
    {
        bail!("rollback is unavailable or the version has changed; refresh the panel");
    }
    Ok(())
}

pub(super) fn run(from: &str, version: &str, say: &mut dyn FnMut(&str)) -> Result<()> {
    let mut install = Install::load()?;
    install.panel_only()?;
    let actual = install.version();
    report(&install, Phase::Running, version, &actual, "");
    let result = (|| -> Result<()> {
        let root = Path::new(DIR);
        // This file is outside data/panel: the container cannot supply an image.
        let raw = fs::read(root.join(RECORD))?;
        if raw.len() > 4096 {
            bail!("invalid rollback record");
        }
        let mut c: Candidate = serde_json::from_slice(&raw)?;
        validate(&c, &actual, from, version)?;
        // A pruned image is the usual reason; say so instead of "does not start".
        if !docker::image_present(&c.image) {
            bail!("the image of mikan {version} is no longer on this server (docker image prune removes it); rollback is unavailable");
        }
        if setup::image_version(&c.image)?.trim_start_matches('v') != version {
            bail!("the previous image does not match its recorded version");
        }
        let current_image = image_id().context("the current image could not be retained for recovery")?;
        // No migrations and no database restore: the old image must run on today's
        // data. A failed start returns to the image that was running before rollback.
        let gate = backup::backup_as(backup::PRE_UPDATE, say).context("the backup before rollback failed; the panel was not stopped")?;
        let _critical = signals::critical();
        let mut recovery = EnvFile::from_text(root.join(".env"), &install.env.render());
        recovery.set("MIKAN_IMAGE", &current_image)?;
        let env = recovery.render();
        let compose = fs::read_to_string(root.join("compose.yaml"))?;
        docker::compose_run(&["stop", "panel"])?;
        let saved = match backup::archive_stopped(root, backup::PRE_UPDATE, true) {
            Ok(saved) => {
                let _ = fs::remove_file(gate);
                saved
            }
            Err(e) => {
                let before = Before { env: &env, compose: &compose, saved: &gate, current: &actual };
                return Err(roll_back(&mut install, &before, &format!("the stopped panel could not be backed up: {e:#}")));
            }
        };
        say(&format!("Backup: {}", saved.display()));
        let before = Before { env: &env, compose: &compose, saved: &saved, current: &actual };
        let switched = (|| -> Result<()> {
            install.env.set("MIKAN_IMAGE", &c.image)?;
            install.env.set("MIKAN_VERSION", version)?;
            install.env.save()?;
            start(&install, root)
        })();
        if let Err(e) = switched {
            return Err(roll_back(&mut install, &before, &format!("rollback to {version} did not start: {e:#}")));
        }
        c.current = version.to_owned();
        c.available = false;
        c.reason = "used".into();
        if let Err(e) = panelfs::write_root(&root.join(RECORD), &serde_json::to_vec(&c)?).and_then(|_| publish(&c)) {
            say(&format!("The version was restored, but its rollback record could not be refreshed: {e:#}"));
        }
        say(&format!("mikan {version} is running; current PostgreSQL data were preserved."));
        Ok(())
    })();
    match &result {
        Ok(()) => report(&install, Phase::Ok, version, &actual, ""),
        Err(e) => report(&install, Phase::Failed, version, &actual, &format!("{e:#}")),
    }
    // An adapter request may have deferred its wake-up to this operation, just as
    // it does to an ordinary update. Do not strand it when auto-updates are off.
    if addon::pending()
        && let Err(e) = addon::apply_here(say)
    {
        say(&format!("Addons: {e:#}"));
    }
    result
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn a_request_cannot_select_an_image_or_a_stale_version() {
        assert!(matches!(
            asked(&serde_json::json!({"do":"rollback","from":"0.5.0.5-tt.9","version":"0.5.0.5-tt.8","image":"evil"})),
            Asked::Rollback(_, _)
        ));
        assert!(matches!(asked(&serde_json::json!({"do":"rollback","from":"dev","version":"evil"})), Asked::Garbage(_)));
        let mut c = Candidate {
            current: "0.5.0.5-tt.9".into(),
            version: "0.5.0.5-tt.8".into(),
            image: format!("sha256:{}", "a".repeat(64)),
            available: true,
            reason: String::new(),
            at: String::new(),
        };
        assert!(validate(&c, &c.current, &c.current, &c.version).is_ok());
        assert!(validate(&c, "0.5.0.5-tt.10", &c.current, &c.version).is_err());
        assert!(validate(&c, &c.current, &c.current, "0.5.0.5-tt.7").is_err());
        c.available = false;
        assert!(validate(&c, &c.current, &c.current, &c.version).is_err());
    }

    #[test]
    fn only_a_known_unchanged_schema_and_previous_image_enable_rollback() {
        let current = "0.5.0.5-tt.9";
        let previous = "0.5.0.5-tt.8";
        let image = || Some(format!("sha256:{}", "a".repeat(64)));
        assert!(candidate(current, previous, image(), true).available);
        let migrated = candidate(current, previous, image(), false);
        assert!(!migrated.available);
        assert_eq!(migrated.reason, "schema_changed");
        let missing = candidate(current, previous, None, true);
        assert!(!missing.available);
        assert_eq!(missing.reason, "image_missing");
        for previous in [current, "0.5.0.5-tt.10", "dev", ""] {
            assert!(!candidate(current, previous, image(), true).available);
        }
        assert!(!candidate("dev", previous, image(), true).available);
        assert!(!candidate(current, previous, Some("ghcr.io/evil/panel:latest".into()), true).available);
    }

    #[test]
    fn a_fork_rollback_requires_both_schema_reports() {
        let upstream = "PostgreSQL schema version: 18 -> 18\n";
        let fork = "0.5.0.5-tt.9";
        assert!(!schema_unchanged(upstream, fork));
        assert!(schema_unchanged(&format!("{upstream}Kinkan schema version: 3 -> 3\n"), fork));
        assert!(!schema_unchanged(&format!("{upstream}Kinkan schema version: 2 -> 3\n"), fork));
        assert!(!schema_unchanged(&format!("{upstream}Kinkan schema version: unknown -> unknown\n"), fork));
        assert!(!schema_unchanged("PostgreSQL schema version: 18 -> 19\nKinkan schema version: 3 -> 3\n", fork));
        assert!(schema_unchanged(upstream, "0.5.0.4"));
    }

    #[test]
    fn the_panel_view_has_no_image_and_replaces_links() {
        use std::os::unix::fs::symlink;
        let path = std::env::temp_dir().join(format!("kinkan-rollback-view-{}", std::process::id()));
        fs::create_dir_all(&path).unwrap();
        let secret = path.join("secret");
        fs::write(&secret, "root's file").unwrap();
        symlink(&secret, path.join("rollback.json")).unwrap();
        let dir = panelfs::Dir::open(&path, ".", false).unwrap().unwrap();
        let c = candidate("0.5.0.5-tt.9", "0.5.0.5-tt.8", Some(format!("sha256:{}", "a".repeat(64))), true);
        publish_in(&dir, &c).unwrap();
        let view: serde_json::Value = serde_json::from_slice(&dir.read("rollback.json", 4096).unwrap().unwrap()).unwrap();
        assert!(view["image"].is_null());
        assert_eq!(view["version"], c.version);
        assert_eq!(view["available"], true);
        assert_eq!(fs::read_to_string(secret).unwrap(), "root's file");
        fs::remove_dir_all(path).unwrap();
    }
}
