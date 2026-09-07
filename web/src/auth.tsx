import {
  createContext,
  ReactNode,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react";
import { Navigate, useLocation } from "react-router-dom";
import {
  api,
  ApiError,
  clearAdminKey,
  getAdminKey,
  getStoredUser,
  setAdminKey,
  setStoredUser,
} from "./api/client";
import type { LoginResponse } from "./api/types";

interface AuthCtx {
  authed: boolean;
  checking: boolean;
  user: LoginResponse["user"] | null;
  login: (key: string) => Promise<void>;
  loginWithPassword: (email: string, pass: string) => Promise<void>;
  logout: () => void;
}

const Ctx = createContext<AuthCtx>({
  authed: false,
  checking: true,
  user: null,
  login: async () => {},
  loginWithPassword: async () => {},
  logout: () => {},
});

export function AuthProvider({ children }: { children: ReactNode }) {
  const [authed, setAuthed] = useState<boolean>(getAdminKey() !== null);
  const [checking, setChecking] = useState(true);
  const [user, setUser] = useState<LoginResponse["user"] | null>(getStoredUser());

  useEffect(() => {
    let alive = true;
    if (getAdminKey()) {
      // Re-verify stored session or root key
      api
        .getCurrentUser()
        .then((u) => {
          if (alive) {
            setAuthed(true);
            const userObj = {
              id: u.id,
              email: u.email,
              name: u.name,
              role: u.role,
              is_root: u.is_root ?? false,
              groups: [],
            };
            setUser(userObj);
            setStoredUser(userObj);
          }
        })
        .catch(() => {
          // Fallback verify via listUsers for emergency root admin key
          api
            .listUsers()
            .then(() => {
              if (alive) {
                setAuthed(true);
                const rootUser = {
                  id: "00000000-0000-0000-0000-000000000001",
                  email: "root@gateway",
                  name: "Root Administrator",
                  role: "admin",
                  is_root: true,
                  groups: [],
                };
                setUser(rootUser);
                setStoredUser(rootUser);
              }
            })
            .catch(() => {
              clearAdminKey();
              if (alive) {
                setAuthed(false);
                setUser(null);
              }
            })
            .finally(() => {
              if (alive) setChecking(false);
            });
        })
        .finally(() => {
          if (alive) setChecking(false);
        });
    } else {
      setChecking(false);
    }
    return () => {
      alive = false;
    };
  }, []);

  const login = useCallback(async (key: string) => {
    setAdminKey(key);
    try {
      await api.listUsers();
      setAuthed(true);
      const rootUser = {
        id: "00000000-0000-0000-0000-000000000001",
        email: "root@gateway",
        name: "Root Administrator",
        role: "admin",
        is_root: true,
        groups: [],
      };
      setUser(rootUser);
      setStoredUser(rootUser);
    } catch (e) {
      clearAdminKey();
      setAuthed(false);
      setUser(null);
      throw e instanceof ApiError ? e : new ApiError(0, "Cannot reach admin API");
    }
  }, []);

  const loginWithPassword = useCallback(async (email: string, pass: string) => {
    try {
      const res = await api.login(email, pass);
      setAdminKey(res.token);
      setStoredUser(res.user);
      setUser(res.user);
      setAuthed(true);
    } catch (e) {
      clearAdminKey();
      setAuthed(false);
      setUser(null);
      throw e instanceof ApiError ? e : new ApiError(0, "Login failed");
    }
  }, []);

  const logout = useCallback(() => {
    clearAdminKey();
    setUser(null);
    setAuthed(false);
  }, []);

  return (
    <Ctx.Provider value={{ authed, checking, user, login, loginWithPassword, logout }}>
      {children}
    </Ctx.Provider>
  );
}

export function useAuth(): AuthCtx {
  return useContext(Ctx);
}

export function RequireAuth({ children }: { children: ReactNode }) {
  const { authed, checking } = useAuth();
  const location = useLocation();

  if (checking) {
    return (
      <div className="login">
        <div className="spinner" />
      </div>
    );
  }
  if (!authed) {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }
  return <>{children}</>;
}
