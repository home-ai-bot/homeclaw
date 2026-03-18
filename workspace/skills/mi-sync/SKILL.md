---
name: mi-sync
description: Sync Xiaomi Mi Home devices, rooms, and homes. Use when the user wants to sync/connect/import Xiaomi smart home devices, set up Mi Home integration, or refresh device/room lists from Mi Home cloud. Handles the full OAuth2 auth flow and device discovery automatically.
---

# Mi Home Sync

Orchestrates the full Xiaomi Mi Home sync workflow using atomic tools.

## Workflow

Execute steps in order. Each step depends on the previous.

### Step 1 — Load cached token

Call `mi_get_account`.

- If `account` is non-null → token valid, skip to Step 4.
- If `account` is null → proceed to Step 2.

### Step 2 — Obtain OAuth authorization

Call `mi_get_oauth_url`. Present the returned `url` to the user:

> Please open this URL in your browser and complete Xiaomi login:
> `<url>`
> After login you will be redirected to a callback URL. Please paste the full callback URL or just the `code_value` parameter.

Wait for the user to provide the code.

### Step 3 — Exchange code for token

Call `mi_get_access_token` with the `code` the user provided.

On success the token is saved. Continue to Step 4.

### Step 4 — Check home binding

Call `mi_get_account` again to read the latest account.

- If `home_id` is non-empty → skip to Step 5.
- If `home_id` is empty → call `mi_sync_homes` to retrieve the home list.
  - If only 1 home → call `mi_update_home` with that home automatically.
  - If more than 1 home → present the list and ask the user which home to bind. Call `mi_update_home` with the chosen `home_id` and `home_name`.

### Step 5 — Sync rooms

Call `mi_sync_rooms` with the bound `home_id`.

Report result: rooms created / updated / removed.

### Step 6 — Sync devices

Call `mi_sync_devices` with the same `home_id`.

Report result: devices created / updated / removed.

## Final Summary

After all steps complete, summarize in natural language:

- Home bound: `<home_name>` (`<home_id>`)
- Rooms: X created, Y updated, Z removed
- Devices: X created, Y updated, Z removed

## Error Handling

| Situation | Action |
|---|---|
| `mi_get_access_token` fails (invalid code) | Ask user to re-paste the code or restart from Step 2 |
| `mi_sync_homes` returns 0 homes | Inform user no homes found in Mi Home account |
| `mi_sync_rooms` / `mi_sync_devices` fails | Report error, suggest re-running the sync |
