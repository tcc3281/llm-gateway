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
  setAdminKey,
} from "./api/client";

interface AuthCtx {
  authed: boolean;
  checking: boolean;
  login: (key: string) => Promise<void>;
  logout: () => void;
}

const Ctx = createContext<AuthCtx>({
  authed: false,
  checking: true,
  login: async () => {},
  logout: () => {},
});

export function AuthProvider({ children }: { children: ReactNode }) {
  const [authed, setAuthed] = useState<boolean>(getAdminKey() !== null);
  const [checking, setChecking] = useState(true);

  useEffect(() => {
    let alive = true;
    if (getAdminKey()) {
      // Re-verify a stored key on refresh.
      api
        .listUsers()
        .then(() => {
          if (alive) setAuthed(true);
        })
        .catch(() => {
          clearAdminKey();
          if (alive) setAuthed(false);
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
    } catch (e) {
      clearAdminKey();
      setAuthed(false);
      throw e instanceof ApiError ? e : new ApiError(0, "Cannot reach admin API");
    }
  }, []);

  const logout = useCallback(() => {
    clearAdminKey();
    setAuthed(false);
  }, []);

  return (
    <Ctx.Provider value={{ authed, checking, login, logout }}>
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
