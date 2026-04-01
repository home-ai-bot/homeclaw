import {
  Navigate,
  Outlet,
  createFileRoute,
  useRouterState,
} from "@tanstack/react-router"

export const Route = createFileRoute("/smart-home")({
  component: SmartHomeLayout,
})

function SmartHomeLayout() {
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })

  if (pathname === "/smart-home") {
    return <Navigate to="/smart-home/tuya" />
  }

  return <Outlet />
}
