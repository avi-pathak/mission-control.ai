import { create } from 'zustand';
import { api, UnauthorizedError, type AuthUser } from '../lib/api';
import { getToken, setToken } from '../lib/config';

interface AuthState {
  token: string;
  user: AuthUser | null;
  org: { id: string; name: string; slug: string } | null;
  ready: boolean; // initial /me check complete
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, orgName?: string) => Promise<void>;
  acceptInvite: (token: string, password: string) => Promise<void>;
  logout: () => void;
  loadMe: () => Promise<void>;
}

export const useAuthStore = create<AuthState>((set) => ({
  token: getToken(),
  user: null,
  org: null,
  ready: false,

  login: async (email, password) => {
    const res = await api.login(email, password);
    setToken(res.token);
    set({ token: res.token, user: res.user });
  },

  register: async (email, password, orgName) => {
    const res = await api.register(email, password, orgName);
    setToken(res.token);
    set({ token: res.token, user: res.user });
  },

  acceptInvite: async (token, password) => {
    const res = await api.acceptInvite(token, password);
    setToken(res.token);
    set({ token: res.token, user: res.user });
  },

  logout: () => {
    setToken('');
    set({ token: '', user: null, org: null });
    // Tear down the live WebSocket + cached tenant data.
    void import('./live').then((m) => m.useLiveStore.getState().disconnect());
  },

  loadMe: async () => {
    if (!getToken()) {
      set({ ready: true });
      return;
    }
    try {
      const me = await api.me();
      set({ user: me.user, org: me.org, ready: true });
    } catch (err) {
      // Only log out on a confirmed 401 (invalid/expired token). Transient
      // network errors or aborted requests (e.g. rapid refresh) must NOT clear
      // the session — keep the token and let the app retry.
      if (err instanceof UnauthorizedError) {
        setToken('');
        set({ token: '', user: null, org: null, ready: true });
      } else {
        set({ ready: true });
      }
    }
  },
}));
