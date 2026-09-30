#!/usr/bin/env python3
"""Sync swagger JSON files from gen/ to docs/api/ directories."""

import shutil
from pathlib import Path

ROOT = Path(__file__).parent.parent.parent
GEN = ROOT / "gen" / "evey" / "v1"
API = ROOT / "docs" / "api"

MAPPING = {
    "public": [
        "library", "playback", "user", "collection", "favorite", "feed",
        "playlist", "rating", "share", "syncplay", "music",
        "photo", "push", "notification", "media_request",
    ],
    "admin": [
        "admin_system", "admin_media", "admin_session", "admin_task",
        "server_config", "metadata", "transcoding", "transcode_cache",
        "backup", "activity", "library_access", "remote_access",
        "parental", "playback_settings", "mkv", "admin_intro",
        "backup_schedule", "presence",
    ],
    "plugins": ["admin_plugin", "webhook", "plugin_prefs"],
}

for category, services in MAPPING.items():
    dest = API / category
    dest.mkdir(parents=True, exist_ok=True)
    for svc in services:
        src = GEN / f"{svc}.swagger.json"
        if src.exists():
            shutil.copy2(src, dest / src.name)
            print(f"  {category}/{src.name}")
        else:
            print(f"  {category}/{src.name} - NOT FOUND")
