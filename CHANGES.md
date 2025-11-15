# ironDHCP v1.0.0 Release Changes

## Version Update
- Updated from v0.2.0 to v1.0.0
- Updated banner and version strings in `cmd/godhcp/main.go`
- Updated `web/package.json` version

## Mobile-Responsive Web UI
### Layout Improvements
- Added hamburger menu for mobile devices with slide-out sidebar
- Responsive navigation with overlay for mobile screens
- Mobile-friendly header bar visible on small screens
- Proper z-index layering for mobile menu components
- Responsive padding: `p-4` on mobile, `p-6` on tablet, `p-8` on desktop

### Activity Page Improvements
- Responsive header layout (stacked on mobile, row on desktop)
- Stats grid: 2 columns on mobile, 4 on desktop
- Adjusted text sizes for mobile readability
- Variable height event feed: 400px mobile, 500px tablet, 600px desktop

## Activity Stream Fix
- Fixed authentication for Server-Sent Events (SSE)
- Updated `Activity.tsx` to use `api.createActivityLogStream()` with proper auth
- Enhanced `AuthMiddleware` to support query parameter tokens for SSE compatibility
  - Checks `Authorization` header first (for regular API calls)
  - Falls back to `?token=` query parameter (for EventSource compatibility)

## Embedded Resources - Self-Contained Binary
### SQL Migrations Embedded
- Moved migrations from `/migrations/` to `/internal/storage/migrations/`
- Added `//go:embed migrations/*.sql` directive in `storage/init.go`
- Updated migration loading to use `migrationsFS.ReadFile()` instead of `os.ReadFile()`
- Migrations now fully embedded in binary - no external SQL files needed

### Web UI Already Embedded
- Frontend built to `internal/api/dist/`
- Embedded via `//go:embed all:dist` in `internal/api/handlers.go`
- All static assets included in binary

### Dockerfile Updates
- Removed separate migration file copying in final stage
- Added comment noting migrations and frontend are embedded
- Simplified runtime image - only needs binary and config.yaml

## Build Results
- Binary size: 23MB (includes web UI + SQL migrations)
- **Only external file needed: config.yaml**
- Verified embedded content via strings analysis:
  - SQL: CREATE TABLE, ALTER TABLE statements present
  - Web: index.html and API routes present

## Files Changed
1. `cmd/godhcp/main.go` - Version update
2. `web/package.json` - Version update
3. `web/src/components/Layout.tsx` - Mobile responsive layout
4. `web/src/pages/Activity.tsx` - Mobile responsive + auth fix
5. `internal/api/auth.go` - SSE query parameter auth support
6. `internal/storage/init.go` - Embedded migrations
7. `internal/storage/migrations/*.sql` - Moved from root
8. `Dockerfile` - Simplified for embedded resources
