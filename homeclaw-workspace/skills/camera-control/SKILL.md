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
- commandJson: {"brand":"any","method":"listCameras"}
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

Plus image content in MediaResult if `return_image: true`

Report the analysis result to the user in natural language.

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
