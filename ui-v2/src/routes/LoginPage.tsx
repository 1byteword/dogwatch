import { createSignal } from "solid-js";
import { useAuth } from "../domains/auth/context";
import dogwatchLogo from "../assets/dogwatch-logo.png";

export default function LoginPage() {
  const { login } = useAuth();
  const [email, setEmail] = createSignal("");
  const [password, setPassword] = createSignal("");
  const [error, setError] = createSignal("");
  const [submitting, setSubmitting] = createSignal(false);

  async function handleSubmit(e: Event) {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      await login(email(), password());
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div class="login-page">
      <div class="login-glow login-glow-1" />
      <div class="login-glow login-glow-2" />

      <form class="login-card" onSubmit={handleSubmit}>
        <div class="login-header">
          <img src={dogwatchLogo} alt="dogwatch" class="login-logo" />
          <div class="login-header-text">
            <h1>dogwatch</h1>
            <span>eBPF observability platform</span>
          </div>
        </div>

        <div class="login-divider" />

        {error() && (
          <div class="login-error" role="alert">
            <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
              <circle cx="8" cy="8" r="7" stroke="currentColor" stroke-width="1.5" />
              <path d="M8 4.5v4M8 10.5v.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
            </svg>
            {error()}
          </div>
        )}

        <div class="login-field">
          <label for="login-email">Email</label>
          <input
            id="login-email"
            type="email"
            class="login-input"
            value={email()}
            onInput={(e) => setEmail(e.currentTarget.value)}
            placeholder="admin@localhost"
            required
            autofocus
          />
        </div>

        <div class="login-field">
          <label for="login-password">Password</label>
          <input
            id="login-password"
            type="password"
            class="login-input"
            value={password()}
            onInput={(e) => setPassword(e.currentTarget.value)}
            placeholder="Enter password"
            required
          />
        </div>

        <button type="submit" class="login-submit" disabled={submitting()}>
          {submitting() ? (
            <span class="login-spinner" />
          ) : (
            "Sign in"
          )}
        </button>

        <p class="login-footer">
          Single binary. Zero config. Your infrastructure.
        </p>
      </form>

      <style>{`
        .login-page {
          display: flex;
          align-items: center;
          justify-content: center;
          min-height: 100vh;
          position: relative;
          overflow: hidden;
        }

        .login-glow {
          position: absolute;
          border-radius: 50%;
          filter: blur(120px);
          pointer-events: none;
        }
        .login-glow-1 {
          width: 500px;
          height: 500px;
          top: -180px;
          left: -100px;
          background: rgba(204, 255, 0, 0.04);
        }
        .login-glow-2 {
          width: 400px;
          height: 400px;
          bottom: -140px;
          right: -80px;
          background: rgba(173, 77, 46, 0.06);
        }

        .login-card {
          position: relative;
          display: flex;
          flex-direction: column;
          gap: 1.25rem;
          width: 100%;
          max-width: 400px;
          padding: 2.5rem;
          border: 1px solid var(--v2-border);
          border-radius: var(--v2-radius-lg);
          background:
            linear-gradient(150deg, rgba(14, 14, 14, 0.95), rgba(8, 8, 8, 0.94)),
            repeating-linear-gradient(32deg, rgba(255, 255, 255, 0.01) 0 2px, transparent 2px 8px);
          box-shadow:
            var(--v2-shadow-md),
            0 0 0 1px rgba(255, 255, 255, 0.03) inset;
        }

        .login-header {
          display: flex;
          align-items: center;
          gap: 1rem;
        }
        .login-logo {
          width: 52px;
          height: 52px;
          flex-shrink: 0;
        }
        .login-header-text {
          display: flex;
          flex-direction: column;
          gap: 2px;
        }
        .login-header-text h1 {
          margin: 0;
          font-size: 1.35rem;
          font-weight: 700;
          letter-spacing: 0.04em;
          color: var(--v2-text);
        }
        .login-header-text span {
          font-size: 0.72rem;
          text-transform: uppercase;
          letter-spacing: 0.1em;
          color: var(--v2-text-muted);
        }

        .login-divider {
          height: 1px;
          background: linear-gradient(90deg, transparent, var(--v2-border-strong), transparent);
        }

        .login-error {
          display: flex;
          align-items: center;
          gap: 0.5rem;
          padding: 0.65rem 0.85rem;
          border-radius: var(--v2-radius-md);
          border: 1px solid rgba(255, 77, 85, 0.25);
          background: rgba(255, 77, 85, 0.06);
          color: var(--v2-error);
          font-size: 0.82rem;
        }

        .login-field {
          display: flex;
          flex-direction: column;
          gap: 0.4rem;
        }
        .login-field label {
          font-size: 0.72rem;
          text-transform: uppercase;
          letter-spacing: 0.08em;
          color: var(--v2-text-muted);
        }
        .login-input {
          width: 100%;
          border: 1px solid var(--v2-border-strong);
          background: rgba(255, 255, 255, 0.02);
          color: var(--v2-text);
          border-radius: var(--v2-radius-md);
          padding: 0.7rem 0.85rem;
          font-size: 0.88rem;
          font-family: var(--v2-font-ui);
          transition: border-color 150ms ease;
        }
        .login-input:focus {
          outline: none;
          border-color: rgba(204, 255, 0, 0.5);
          box-shadow: 0 0 0 3px rgba(204, 255, 0, 0.08);
        }
        .login-input::placeholder {
          color: rgba(138, 138, 138, 0.5);
        }

        .login-submit {
          width: 100%;
          margin-top: 0.25rem;
          padding: 0.75rem;
          border: 1px solid var(--v2-accent);
          border-radius: var(--v2-radius-md);
          background: linear-gradient(145deg, var(--v2-accent), #a9d600);
          color: #080808;
          font-family: var(--v2-font-ui);
          font-size: 0.82rem;
          font-weight: 700;
          letter-spacing: 0.06em;
          text-transform: uppercase;
          cursor: pointer;
          transition: opacity 150ms ease, transform 80ms ease;
          display: flex;
          align-items: center;
          justify-content: center;
          min-height: 44px;
        }
        .login-submit:hover:not(:disabled) {
          opacity: 0.92;
        }
        .login-submit:active:not(:disabled) {
          transform: scale(0.985);
        }
        .login-submit:disabled {
          opacity: 0.6;
          cursor: wait;
        }
        .login-submit:focus-visible {
          outline: 2px solid var(--v2-accent);
          outline-offset: 2px;
        }

        .login-spinner {
          display: block;
          width: 18px;
          height: 18px;
          border: 2px solid rgba(8, 8, 8, 0.3);
          border-top-color: #080808;
          border-radius: 50%;
          animation: login-spin 0.6s linear infinite;
        }
        @keyframes login-spin {
          to { transform: rotate(360deg); }
        }

        .login-footer {
          margin: 0;
          text-align: center;
          font-size: 0.7rem;
          letter-spacing: 0.06em;
          text-transform: uppercase;
          color: rgba(138, 138, 138, 0.4);
        }
      `}</style>
    </div>
  );
}
