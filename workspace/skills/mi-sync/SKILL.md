---
name: mi-sync
description: Sync Xiaomi Mi Home devices. Use when the user wants to sync/import Xiaomi smart home devices or refresh device list from Mi Home cloud.
---

# Mi Home Sync

## Workflow

### Step 1 — Sync devices

Call `mi_sync_devices` (no parameters needed).

The tool automatically:
- Fetches homes from cloud if none cached
- Auto-selects if only one home
- Syncs devices and rooms

### Step 2 — Handle multiple homes (if needed)

If `mi_sync_devices` returns a home list asking user to choose:

1. Present the home list to user
2. User selects a home
3. Call `hc_set_current_home` with the chosen `home_id` and `brand` = "xiaomi"
4. Call `mi_sync_devices` again
