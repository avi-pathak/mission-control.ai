import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { RouterProvider, createRouter } from '@tanstack/react-router';
import { StrictMode, useEffect } from 'react';
import { createRoot } from 'react-dom/client';
import './index.css';
import { AuthPage } from './pages/AuthPage';
import { routeTree } from './router';
import { useAuthStore } from './store/auth';

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 5000, refetchOnWindowFocus: false, retry: false } },
});

const router = createRouter({ routeTree, context: { queryClient } });

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}

function Root() {
  const ready = useAuthStore((s) => s.ready);
  const token = useAuthStore((s) => s.token);
  const loadMe = useAuthStore((s) => s.loadMe);

  useEffect(() => {
    loadMe();
  }, [loadMe]);

  // Public accept-invite route, or no token → auth screen.
  const path = window.location.pathname;
  if (path.startsWith('/accept-invite') || !token) {
    return <AuthPage />;
  }
  if (!ready) {
    return <div className="flex h-screen items-center justify-center text-sm text-zinc-500">Loading…</div>;
  }
  return <RouterProvider router={router} />;
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <Root />
    </QueryClientProvider>
  </StrictMode>,
);

// Register the service worker for PWA install + Web Push notifications.
if ('serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch(() => {
      /* SW registration is best-effort; the app works without it. */
    });
  });
}
