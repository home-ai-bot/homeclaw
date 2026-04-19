# Xiaomi & Tuya Page Redesign Plan

## Context

The Xiaomi and Tuya pages currently only have basic authorization sections and use the SmartHomeLayout wrapper. We need to redesign them into a structured 4-section layout from top to bottom:
1. **Authorization** - Login/token management with logged-in status display
2. **Family Information** - Sync homes button, home list with selection, confirmation on change
3. **Device List** - Sync devices button, device list (basic info only, no operations)
4. **Local Video Settings** - Placeholder for future implementation

All sync operations will use WebSocket tool calls (hc_cli) rather than HTTP APIs. Device lists will show basic info in a simple list format without operation buttons.

## Key Files to Modify

### Frontend Components
- `web/frontend/src/homeclaw/components/xiaomi-page.tsx` - Complete redesign
- `web/frontend/src/homeclaw/components/tuya-page.tsx` - Complete redesign

### Frontend Stores (Extend)
- `web/frontend/src/homeclaw/store/xiaomi.ts` - Add home/device state
- `web/frontend/src/homeclaw/store/tuya.ts` - Add home/device state

### Frontend i18n (Extend)
- `web/frontend/src/i18n/locales/homeclaw/en.json` - Add new translation keys
- `web/frontend/src/i18n/locales/homeclaw/zh.json` - Add new translation keys

## Implementation Steps

### Step 1: Extend Store State

#### Xiaomi Store (`xiaomi.ts`)
Add to state interface:
```typescript
interface XiaomiStoreState {
  // Existing fields...
  isLoggedIn: boolean
  isLoading: boolean
  userId: string | null
  error: string | null
  loginStep: "login" | "captcha" | "verify" | "success"
  captchaImage: string | null
  verifyTarget: string | null
  verifyType: "phone" | "email" | null
  
  // NEW: Home/Family state
  homes: Array<{ id: string; name: string }>
  selectedHomeId: string | null
  isSyncingHomes: boolean
  
  // NEW: Device state
  devices: Array<{
    from_id: string
    name: string
    type: string
    space_name?: string
    online?: boolean
  }>
  isSyncingDevices: boolean
}
```

Add store functions:
- `syncXiaomiHomes()` - Call hc_cli syncHomes via WebSocket, update homes list
- `selectXiaomiHome(homeId: string)` - Set selected home, call hc_cli setCurrentHome
- `syncXiaomiDevices()` - Call hc_cli syncDevices via WebSocket, update device list

#### Tuya Store (`tuya.ts`)
Same structure as Xiaomi, add:
- `homes`, `selectedHomeId`, `isSyncingHomes`
- `devices`, `isSyncingDevices`

Add store functions:
- `syncTuyaHomes()`
- `selectTuyaHome(homeId: string)`
- `syncTuyaDevices()`

### Step 2: Create Reusable Section Components

Create shared components that both pages can use:

#### File: `web/frontend/src/homeclaw/components/home-section.tsx`

**Props:**
```typescript
interface HomeSectionProps {
  homes: Array<{ id: string; name: string }>
  selectedHomeId: string | null
  isSyncing: boolean
  onSync: () => void
  onSelect: (homeId: string) => void
}
```

**UI:**
- Card with title "Family/Home Information"
- "Sync Homes" button at top
- Loading spinner while syncing
- List of homes with radio/checkbox selection
- When selection changes, show confirmation dialog before proceeding

#### File: `web/frontend/src/homeclaw/components/device-list-section.tsx`

**Props:**
```typescript
interface DeviceListSectionProps {
  devices: Array<{
    from_id: string
    name: string
    type: string
    space_name?: string
    online?: boolean
  }>
  isSyncing: boolean
  onSync: () => void
}
```

**UI:**
- Card with title "Device List"
- "Sync Devices" button at top
- Loading spinner while syncing
- Simple list display showing:
  - Device name
  - Device type (badge)
  - Room/space name (if available)
  - Online status indicator (green/gray dot)
- No operation buttons

#### File: `web/frontend/src/homeclaw/components/video-settings-section.tsx`

**UI:**
- Card with title "Local Video Settings"
- Placeholder text: "Coming soon..."
- Empty state for future implementation

### Step 3: Redesign Xiaomi Page

**Structure:**
```tsx
<SmartHomeLayout ...>
  {/* Section 1: Authorization */}
  <AuthSection
    isLoggedIn={state.isLoggedIn}
    userId={state.userId}
    loginStep={state.loginStep}
    // ... login form state and handlers
  />
  
  {/* Section 2: Family Information (only show if logged in) */}
  {state.isLoggedIn && (
    <HomeSection
      homes={state.homes}
      selectedHomeId={state.selectedHomeId}
      isSyncing={state.isSyncingHomes}
      onSync={() => void syncXiaomiHomes()}
      onSelect={(id) => void selectXiaomiHome(id)}
    />
  )}
  
  {/* Section 3: Device List (only show if home selected) */}
  {state.selectedHomeId && (
    <DeviceListSection
      devices={state.devices}
      isSyncing={state.isSyncingDevices}
      onSync={() => void syncXiaomiDevices()}
    />
  )}
  
  {/* Section 4: Video Settings (placeholder) */}
  <VideoSettingsSection />
</SmartHomeLayout>
```

**Auth Section Integration:**
- Keep existing login flow (username/password → captcha → verify)
- When logged in, show user ID and logout button
- Simplify the success state display

### Step 4: Redesign Tuya Page

Same structure as Xiaomi page, but:
- Auth section handles both token and credentials authentication
- Keep existing dual auth method UI
- Use Tuya-specific store functions

### Step 5: Add WebSocket Helper

Create helper for calling hc_cli from stores:

#### File: `web/frontend/src/homeclaw/api/home-sync.ts`

```typescript
// Call hc_cli to sync homes
export async function syncHomesViaWS(brand: string): Promise<{
  success: boolean
  homes?: Array<{ id: string; name: string }>
  error?: string
}>

// Call hc_cli to set current home
export async function setCurrentHomeViaWS(
  brand: string,
  homeId: string
): Promise<{ success: boolean; error?: string }>

// Call hc_cli to sync devices
export async function syncDevicesViaWS(
  brand: string,
  homeId: string
): Promise<{
  success: boolean
  devices?: Array<{...}>
  error?: string
}>
```

These will use `deviceControlWS.sendAndWait()` with tool call format:
```json
{
  "type": "message.send",
  "session_id": "device-control",
  "payload": {
    "content": "tool:hc_cli {\"brand\":\"xiaomi\",\"method\":\"syncHomes\"}"
  }
}
```

### Step 6: Add i18n Translations

Add to both `en.json` and `zh.json`:

```json
{
  "home_section": {
    "title": "Family Information",
    "syncHomes": "Sync Homes",
    "syncingHomes": "Syncing homes...",
    "noHomes": "No families found",
    "selectHome": "Select this home",
    "currentHome": "Current home",
    "confirmHomeChange": "Changing the selected home will re-sync devices. Continue?"
  },
  "device_section": {
    "title": "Device List",
    "syncDevices": "Sync Devices",
    "syncingDevices": "Syncing devices...",
    "noDevices": "No devices found",
    "online": "Online",
    "offline": "Offline",
    "room": "Room"
  },
  "video_section": {
    "title": "Local Video Settings",
    "comingSoon": "Coming soon..."
  }
}
```

## Verification Steps

1. **Manual Testing:**
   - Start frontend dev server
   - Navigate to Xiaomi page
   - Test login flow
   - Verify home sync button works and displays homes
   - Select a home, confirm device sync triggers
   - Verify device list displays correctly
   - Repeat for Tuya page with both auth methods

2. **WebSocket Testing:**
   - Open communication log panel
   - Verify syncHomes tool call is sent correctly
   - Verify setCurrentHome tool call is sent on home selection
   - Verify syncDevices tool call includes correct homeId

3. **State Management:**
   - Verify store updates correctly after each operation
   - Verify page maintains state on refresh (if persisted)

4. **i18n Testing:**
   - Switch between English and Chinese
   - Verify all new strings display correctly

## Design Considerations

1. **Conditional Rendering:**
   - Section 2 only shows when logged in
   - Section 3 only shows when a home is selected
   - This creates a progressive disclosure flow

2. **User Confirmation:**
   - When user selects a different home, show confirmation dialog
   - Only proceed with sync after confirmation

3. **Loading States:**
   - Each sync operation shows loading spinner
   - Buttons disabled during sync to prevent double-clicks

4. **Error Handling:**
   - Display errors inline within each section
   - Use existing store error field pattern

5. **Reusability:**
   - HomeSection and DeviceListSection are brand-agnostic
   - Both Xiaomi and Tuya pages use same components
   - Only store functions differ between brands
