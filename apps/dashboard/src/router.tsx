import { createRootRoute, createRoute } from '@tanstack/react-router';
import { AppLayout } from './components/AppLayout';
import { ActivityPage } from './pages/ActivityPage';
import { MachinesPage } from './pages/MachinesPage';
import { OverviewPage } from './pages/OverviewPage';
import { SessionPage } from './pages/SessionPage';
import { SessionsPage } from './pages/SessionsPage';
import { SettingsPage } from './pages/SettingsPage';
import { TerminalPage } from './pages/TerminalPage';
import { TokensPage } from './pages/TokensPage';

const rootRoute = createRootRoute({ component: AppLayout });

const overviewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: OverviewPage,
});

const sessionsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/sessions',
  component: SessionsPage,
});

const sessionRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/sessions/$sessionId',
  component: SessionPage,
});

const machinesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/machines',
  component: MachinesPage,
});

const activityRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/activity',
  component: ActivityPage,
});

const tokensRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/tokens',
  component: TokensPage,
});

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings',
  component: SettingsPage,
});

export interface TerminalSearch {
  machineId: string;
  cwd: string;
  initialText?: string | undefined;
}

const terminalRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/terminal/$ptyId',
  component: TerminalPage,
  validateSearch: (search: Record<string, unknown>): TerminalSearch => ({
    machineId: String(search.machineId ?? ''),
    cwd: String(search.cwd ?? '.'),
    initialText: search.initialText ? String(search.initialText) : undefined,
  }),
});

export const routeTree = rootRoute.addChildren([
  overviewRoute,
  sessionsRoute,
  sessionRoute,
  machinesRoute,
  activityRoute,
  tokensRoute,
  settingsRoute,
  terminalRoute,
]);
