---
name: device-sync
description: Sync smart home devices from any brand (Xiaomi, Tuya, etc.). Use when the user wants to sync/import devices, refresh the device list, or set up a smart home brand for the first time.
---

## Workflow

1. **Get current home** → `hc_cli` getCurrentHome
2. **Sync homes (if needed)** → `hc_cli` syncHomes
3. **User selects home (if multiple)** → `hc_cli` setCurrentHome
4. **Sync devices** → `hc_cli` syncDevices

---

### Step 1 — Get current home

Call `hc_cli` getCurrentHome with `from` = the target brand (e.g. `"xiaomi"` or `"tuya"`).

```
hc_cli
- commandJson: {"brand":"<brand>","method":"getCurrentHome","params":{"from":"<brand>"}}
```

Replace `<brand>` with `"xiaomi"` or `"tuya"`.

- Returns a home → proceed to **Step 4** with the `home_id`
- Error "no homes found" → proceed to **Step 2**
- Error "no current home set" with available homes list → proceed to **Step 3**

---

### Step 2 — Sync homes

```
hc_cli
- commandJson: {"brand":"<brand>","method":"syncHomes"}
```

Replace `<brand>` with `"xiaomi"` or `"tuya"`.

---

### Step 3 —  selects home

1. 1 home returned , goto 3
2. !IMPORTANT more than 2 home ,MUST Present the home list to the user，let user picks a home
3. Call `hc_cli` setCurrentHome with `from_id` = chosen `home_id` and `from` = the brand

```
hc_cli
- commandJson: {"brand":"<brand>","method":"setCurrentHome","params":{"from_id":"<home_id>","from":"<brand>"}}
```

4. Proceed to **Step 4**

---

### Step 4 — Sync devices

```
hc_cli
- commandJson: {"brand":"<brand>","method":"syncDevices","params":{"homeId":"<home_id>"}}
```

This syncs all rooms and devices for the selected home into the local store.

---

## Examples

### Example 1: Sync Xiaomi devices

```
1. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"getCurrentHome\",\"params\":{\"from\":\"xiaomi\"}}"}
   → error: "no homes found"

2. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"syncHomes\"}"}
   → synced 1 homes for brand 'xiaomi': [{"home_id":"123","name":"My Home"}]
   → 1 home, auto-set as current

3. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"syncDevices\",\"params\":{\"homeId\":\"123\"}}"}
   → synced 3 rooms and 12 devices
```

### Example 2: Sync Tuya devices (multiple homes)

```
1. hc_cli {"commandJson":"{\"brand\":\"tuya\",\"method\":\"getCurrentHome\",\"params\":{\"from\":\"tuya\"}}"}
   → error: "no current home set", homes: ["Home (id: a1)", "Office (id: b2)"]

2. Ask user which home to use → user picks "Office" (b2)

3. hc_cli {"commandJson":"{\"brand\":\"tuya\",\"method\":\"setCurrentHome\",\"params\":{\"from_id\":\"b2\",\"from\":\"tuya\"}}"}
   → successfully set home b2 from tuya as current

4. hc_cli {"commandJson":"{\"brand\":\"tuya\",\"method\":\"syncDevices\",\"params\":{\"homeId\":\"b2\"}}"}
   → synced 2 rooms and 8 devices
```

### Example 3: Re-sync when devices are already set up

```
1. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"getCurrentHome\",\"params\":{\"from\":\"xiaomi\"}}"}
   → {"home_id":"123","name":"My Home","from":"xiaomi"}

2. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"syncDevices\",\"params\":{\"homeId\":\"123\"}}"}
   → synced 3 rooms and 12 devices
```

---

## Error Handling

- **Brand not registered**: The brand's credentials are not configured. Inform the user to configure the API key or login first.
- **Auth error / token invalid**: The token has expired. For Xiaomi, ask the user to re-login via the web UI. For Tuya, ask the user to reconfigure the API key.
- **No homes found after syncHomes**: The account has no homes. Ask the user to create a home in the brand's mobile app first.
