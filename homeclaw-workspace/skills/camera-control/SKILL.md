---
name: camera-control
description: Control smart home cameras and perform visual analysis. Use when the user wants to capture camera frames, analyze what the camera sees, check camera feeds, or monitor spaces. Triggers on commands like "what does the camera see?", "is anyone at the door?", "check the baby monitor", "show me [camera name]", or any request to view or analyze camera feeds.
---

## Workflow

1. **Find Camera** — locate the target camera by name or room
2. **Capture & Analyze** — capture frame and optionally perform visual analysis

---

## Step 1 — Find Camera

```
hc_cli
- commandJson: {"brand":"","method":"listCameras"}
```

Returns camera list with `from_id`, `from`, `name`, `type`, `space_name`, `rtsp_url`.

- If user provides `rtsp_url` directly, skip this step
- Match camera by name (e.g., "camera-name", "Living Room Camera") or room/space
- Note the `rtsp_url` for the next step

---

## Step 2 — Capture & Analyze Frame

**If user wants analysis** (e.g., "what do you see?", "is there anyone?"), use `capAnalyze` — more efficient!

```
hc_video
- commandJson: {"method":"capAnalyze","params":{"rtsp_url":"<rtsp_url from step 1>","prompt":"<user's question or analysis request>","return_image":<true/false>}}
```

**If user only wants the image** (no analysis needed,"use  [camera name] capture"):

```
hc_video
- commandJson: {"method":"capImage","params":{"rtsp_url":"<rtsp_url from step 1>","return_image":<true/false>}}
```

**Image delivery:**
- If user wants to receive the image, set `return_image: true`
- Image content will be sent in MediaResult (QQ、dingding can display images directly, no additional steps needed)

**Returns (capAnalyze):**
```json
{"analysis": "Description of what's visible...","file_path": "/tmp/homeclaw_frame_123.jpg"}
```
Plus image content in MediaResult if `return_image: true`

**Returns (capImage):**
```json
{"file_path": "/tmp/homeclaw_frame_123.jpg"}
```
Plus image content in MediaResult if `return_image: true`

Report the analysis result to the user in natural language.

---

## Examples

### Example 1: What does [camera-name] camera see?

```
1. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"listCameras\"}"}
   → {"cameras": [{"from_id": "cam001", "from": "xiaomi", "name": "[camera-name]", "type": "...", "space_name": "Living Room", "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_cam001"}]}

2. hc_video {"commandJson":"{\"method\":\"capAnalyze\",\"params\":{\"rtsp_url\":\"rtsp://127.0.0.1:8554/xiaomi_cam001\",\"prompt\":\"Describe what you see\"}}"}
   → {"analysis": "The room is well-lit. A desk with a laptop is visible. No people present."}
```

### Example 2: Is anyone at the door?

```
1. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"listCameras\"}"}
   → {"cameras": [{"from_id": "cam002", "from": "xiaomi", "name": "Door Camera", "type": "...", "space_name": "Entrance", "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_cam002"}]}

2. hc_video {"commandJson":"{\"method\":\"capAnalyze\",\"params\":{\"rtsp_url\":\"rtsp://127.0.0.1:8554/xiaomi_cam002\",\"prompt\":\"Is there anyone at the door?\",\"return_image\":true}}"}
   → {"analysis": "A person is standing at the door, wearing a blue jacket."}
   → Image sent via MediaResult
```

### Example 3: Capture living room camera frame only

```
1. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"listCameras\"}"}
   → {"cameras": [{"from_id": "cam001", "from": "xiaomi", "name": "Living Room Camera", "type": "...", "space_name": "Living Room", "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_cam001"}]}

2. hc_video {"commandJson":"{\"method\":\"capImage\",\"params\":{\"rtsp_url\":\"rtsp://127.0.0.1:8554/xiaomi_cam001\",\"return_image\":true}}"}
   → {"file_path": "/tmp/homeclaw_frame_123.jpg"}
   → Image sent via MediaResult
```

### Example 4: Check baby monitor

```
1. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"listCameras\"}"}
   → {"cameras": [{"from_id": "cam003", "from": "xiaomi", "name": "Baby Monitor", "type": "...", "space_name": "Bedroom", "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_cam003"}]}

2. hc_video {"commandJson":"{\"method\":\"capAnalyze\",\"params\":{\"rtsp_url\":\"rtsp://127.0.0.1:8554/xiaomi_cam003\",\"prompt\":\"Is the baby sleeping? Describe the room conditions\"}}"}
   → {"analysis": "The baby is sleeping peacefully in the crib. The room has soft lighting and comfortable temperature."}
```

---

## Error Handling

- **Camera not found**: Ask user for more specific camera name or room; list available cameras if needed
- **Camera offline**: Inform user the camera is offline, do not proceed
- **Brand not registered**: Credentials not configured, inform user to run device-sync first
- **RTSP connection failed**: Camera may be offline or go2rtc not running; check prerequisites
- **FFmpeg not available**: FFmpeg must be installed for camera frame capture
- **Vision LLM not configured**: `capAnalyze` requires a vision-capable LLM; inform user if not available
- **Empty analysis**: If capAnalyze returns empty or unclear result, try again with a more specific prompt

---

## Prerequisites

- Cameras must be synced first (use `device-sync` skill)
- go2rtc must be running to serve RTSP streams
- FFmpeg must be installed for frame capture
- Vision-capable LLM must be configured (for `capAnalyze` method)
