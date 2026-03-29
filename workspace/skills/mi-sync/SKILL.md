---
name: mi-sync
description: Sync Xiaomi Mi Home devices. Use when the user wants to sync/import Xiaomi smart home devices or refresh device list from Mi Home cloud.
---

# Mi Home Sync

## Workflow

## Workflow

1. **Get current home** → `hc_get_current_home`
2. **Sync homes (if needed)** → `mi__interanl_1` (get home)
3. **User selects home (if multiple)** → `hc_set_current_home`
4. **Sync devices** → `mi__interanl_2`

### Step 1 — Get current home

Call `hc_get_current_home` with `from` = "xiaomi" to check if a current home is already set.

- If returns a home: proceed to Step 4 with the `home_id`
- If error "no homes found": proceed to Step 2
- If error "no current home set" with available homes list: proceed to Step 3

### Step 2 — Sync homes (if needed)

Call `mi__interanl_1` (no parameters needed).

This fetches all homes from Xiaomi cloud.

- If only 1 home returned: it's auto-set as current, proceed to Step 4
- If multiple homes returned: proceed to Step 3

### Step 3 — User selects home (if multiple)

1. Present the home list to user
2. User selects a home
3. Call `hc_set_current_home` with `from_id` = chosen home_id and `from` = "xiaomi"
4. Proceed to Step 4

### Step 4 — Sync devices

Call `mi__interanl_2` with `homeId` = the current home ID.

This syncs all devices and rooms for the selected home.
