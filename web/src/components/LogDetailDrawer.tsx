import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { UsageLog, UsageLogDetail } from "../api/types";
import { fmtBytes, fmtDate, fmtMoney, fmtNum } from "../lib/format";

export function getStatusInfo(code: number): { label: string; text: string; color: string; bg: string; border: string } {
  const map: Record<number, string> = {
    200: "OK",
    201: "Created",
    204: "No Content",
    400: "Bad Request",
    401: "Unauthorized",
    403: "Forbidden",
    404: "Not Found",
    408: "Request Timeout",
    422: "Unprocessable Entity",
    429: "Too Many Requests",
    500: "Internal Server Error",
    502: "Bad Gateway",
    503: "Service Unavailable",
    504: "Gateway Timeout",
  };

  const text = map[code] || (code >= 200 && code < 300 ? "Success" : code >= 400 && code < 500 ? "Client Error" : "Server Error");

  if (code >= 200 && code < 300) {
    return {
      label: `[${code} ${text}]`,
      text,
      color: "var(--emerald)",
      bg: "rgba(16, 185, 129, 0.12)",
      border: "rgba(16, 185, 129, 0.3)",
    };
  }
  if (code === 429) {
    return {
      label: `[429 ${text}]`,
      text,
      color: "var(--amber)",
      bg: "rgba(245, 158, 11, 0.12)",
      border: "rgba(245, 158, 11, 0.3)",
    };
  }
  if (code >= 400 && code < 500) {
    return {
      label: `[${code} ${text}]`,
      text,
      color: "var(--amber-soft)",
      bg: "rgba(255, 185, 87, 0.12)",
      border: "rgba(255, 185, 87, 0.3)",
    };
  }
  return {
    label: `[${code} ${text}]`,
    text,
    color: "var(--danger)",
    bg: "rgba(248, 113, 113, 0.15)",
    border: "rgba(248, 113, 113, 0.35)",
  };
}

interface LogDetailDrawerProps {
  logId: number | null;
  initialLog?: UsageLog | UsageLogDetail | null;
  onClose: () => void;
}

export default function LogDetailDrawer({ logId, initialLog, onClose }: LogDetailDrawerProps) {
  const [log, setLog] = useState<UsageLogDetail | UsageLog | null>(initialLog ?? null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);

  useEffect(() => {
    if (!logId) {
      setLog(null);
      return;
    }
    setLoading(true);
    setError(null);
    api
      .getLog(logId)
      .then((data) => {
        setLog(data);
      })
      .catch((err) => {
        // Fallback to initial log if detailed fetch fails
        if (initialLog && initialLog.id === logId) {
          setLog(initialLog);
        } else {
          setError(err instanceof Error ? err.message : "Failed to load log detail");
        }
      })
      .finally(() => setLoading(false));
  }, [logId, initialLog]);

  if (!logId) return null;

  const handleCopy = (text: string, label: string) => {
    navigator.clipboard.writeText(text);
    setCopied(label);
    setTimeout(() => setCopied(null), 2000);
  };

  const status = log ? getStatusInfo(log.status_code) : null;
  const isStreaming = Boolean(log && log.ttft_ms > 0);
  const ttft = log?.ttft_ms ?? 0;
  const totalLatency = log?.total_latency_ms ?? 0;
  const genLatency = Math.max(0, totalLatency - ttft);

  // Compute percentages for telemetry timeline
  const ttftPct = totalLatency > 0 && isStreaming ? Math.min(100, Math.max(5, (ttft / totalLatency) * 100)) : 0;
  const genPct = totalLatency > 0 && isStreaming ? Math.max(5, 100 - ttftPct) : 100;

  // Cache savings estimation
  const cacheSavings =
    log && log.cached_input_tokens > 0 && log.prompt_price_used > 0
      ? (log.cached_input_tokens / 1_000_000) * (log.prompt_price_used * 0.5)
      : 0;

  const jsonString = log ? JSON.stringify(log, null, 2) : "";

  const curlString = log
    ? `curl -X POST https://gateway.yourdomain.com/v1/chat/completions \\
  -H "Authorization: Bearer ${log.key_id ? "sk-..." : "YOUR_API_KEY"}" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "${log.model}", "stream": ${isStreaming}}'`
    : "";

  return (
    <div className="drawer-backdrop" onClick={onClose}>
      <aside className="drawer" onClick={(e) => e.stopPropagation()}>
        {/* Drawer Header */}
        <div className="drawer__head">
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 10 }}>
            <span
              style={{
                fontSize: 11,
                fontFamily: "var(--font-mono)",
                fontWeight: 700,
                color: "var(--text-muted)",
                letterSpacing: "0.05em",
                textTransform: "uppercase",
              }}
            >
              INSPECT REQUEST
            </span>
            <button className="drawer__close" onClick={onClose} aria-label="Close">
              <span className="material-symbols-outlined" style={{ fontSize: 18 }}>
                close
              </span>
            </button>
          </div>

          <div style={{ marginBottom: 12 }}>
            <h2
              className="mono"
              style={{
                fontSize: 20,
                fontWeight: 700,
                color: "#fff",
                margin: 0,
                wordBreak: "break-all",
              }}
            >
              req-{log?.id ?? logId}
            </h2>
          </div>

          {status && log && (
            <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
              <span
                className="mono"
                style={{
                  fontSize: 11,
                  padding: "2px 8px",
                  borderRadius: 4,
                  background: status.bg,
                  color: status.color,
                  border: `1px solid ${status.border}`,
                  fontWeight: 600,
                }}
              >
                {status.label}
              </span>
              <span
                className="mono"
                style={{
                  fontSize: 11,
                  padding: "2px 8px",
                  borderRadius: 4,
                  background: "rgba(168, 85, 247, 0.12)",
                  color: "var(--violet)",
                  border: "1px solid rgba(168, 85, 247, 0.3)",
                  fontWeight: 600,
                  textTransform: "uppercase",
                }}
              >
                [{log.provider}]
              </span>
              <span
                className="mono"
                style={{
                  fontSize: 11,
                  padding: "2px 8px",
                  borderRadius: 4,
                  background: "rgba(34, 211, 238, 0.12)",
                  color: "var(--primary)",
                  border: "1px solid rgba(34, 211, 238, 0.3)",
                  fontWeight: 600,
                }}
              >
                [{log.model}]
              </span>
              <span
                className="mono"
                style={{
                  fontSize: 11,
                  padding: "2px 8px",
                  borderRadius: 4,
                  background: "var(--surface-high)",
                  color: "var(--text-muted)",
                  border: "1px solid var(--border)",
                  fontWeight: 500,
                  textTransform: "uppercase",
                }}
              >
                [IN: {log.inbound_format}]
              </span>
            </div>
          )}
        </div>

        {/* Drawer Body */}
        <div className="drawer__body">
          {loading && !log && (
            <div className="empty" style={{ padding: "40px 0" }}>
              Loading request details…
            </div>
          )}

          {error && <div className="error-banner">{error}</div>}

          {log && (
            <>
              {/* Telemetry Timeline */}
              <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                <div
                  style={{
                    fontSize: 11,
                    fontFamily: "var(--font-mono)",
                    fontWeight: 700,
                    color: "var(--text-muted)",
                    letterSpacing: "0.05em",
                    textTransform: "uppercase",
                  }}
                >
                  Telemetry Timeline
                </div>

                <div
                  style={{
                    width: "100%",
                    height: 12,
                    borderRadius: 4,
                    overflow: "hidden",
                    display: "flex",
                    background: "var(--surface-high)",
                  }}
                >
                  {isStreaming ? (
                    <>
                      <div
                        style={{
                          width: `${ttftPct}%`,
                          background: "var(--violet)",
                          transition: "width 0.3s ease",
                        }}
                        title={`TTFT: ${ttft}ms`}
                      />
                      <div
                        style={{
                          width: `${genPct}%`,
                          background: "var(--primary)",
                          transition: "width 0.3s ease",
                        }}
                        title={`Generation Latency: ${genLatency}ms`}
                      />
                    </>
                  ) : (
                    <div
                      style={{
                        width: "100%",
                        background: "var(--primary)",
                      }}
                      title={`Total Latency: ${totalLatency}ms (Non-streaming)`}
                    />
                  )}
                </div>

                <div
                  className="mono"
                  style={{
                    display: "flex",
                    justifyContent: "space-between",
                    fontSize: 12,
                    color: "var(--text-muted)",
                    marginTop: 2,
                  }}
                >
                  {isStreaming ? (
                    <>
                      <span style={{ color: "var(--violet)" }}>TTFT {ttft}ms</span>
                      <span style={{ color: "var(--primary)" }}>GEN {genLatency}ms</span>
                      <span style={{ color: "#dde4e5" }}>TOTAL {totalLatency}ms</span>
                    </>
                  ) : (
                    <>
                      <span style={{ color: "var(--text-faint)" }}>NON-STREAMING</span>
                      <span style={{ color: "#dde4e5" }}>TOTAL {totalLatency}ms</span>
                    </>
                  )}
                </div>
              </div>

              <div style={{ height: 1, background: "var(--border)", width: "100%" }} />

              {/* Token Usage Breakdown */}
              <div>
                <div
                  style={{
                    fontSize: 11,
                    fontFamily: "var(--font-mono)",
                    fontWeight: 700,
                    color: "var(--text-muted)",
                    letterSpacing: "0.05em",
                    textTransform: "uppercase",
                    marginBottom: 12,
                  }}
                >
                  Token Composition
                </div>

                <div
                  style={{
                    display: "grid",
                    gridTemplateColumns: "1fr 1fr",
                    gap: "12px 20px",
                    fontFamily: "var(--font-mono)",
                  }}
                >
                  <div style={{ background: "var(--surface-raised)", padding: "10px 12px", borderRadius: 4, border: "1px solid var(--border)" }}>
                    <div style={{ fontSize: 10, color: "var(--text-muted)", textTransform: "uppercase", marginBottom: 2 }}>
                      Prompt Tokens
                    </div>
                    <div style={{ fontSize: 15, fontWeight: 600, color: "var(--text)" }}>
                      {fmtNum(log.prompt_tokens)}
                    </div>
                  </div>

                  <div style={{ background: "var(--surface-raised)", padding: "10px 12px", borderRadius: 4, border: "1px solid var(--border)" }}>
                    <div style={{ fontSize: 10, color: "var(--text-muted)", textTransform: "uppercase", marginBottom: 2 }}>
                      Completion Tokens
                    </div>
                    <div style={{ fontSize: 15, fontWeight: 600, color: "var(--text)" }}>
                      {fmtNum(log.completion_tokens)}
                    </div>
                  </div>

                  <div style={{ background: "var(--surface-raised)", padding: "10px 12px", borderRadius: 4, border: "1px solid var(--border)" }}>
                    <div style={{ fontSize: 10, color: "var(--text-muted)", textTransform: "uppercase", marginBottom: 2 }}>
                      Cached Input
                    </div>
                    <div style={{ fontSize: 15, fontWeight: 600, color: log.cached_input_tokens > 0 ? "var(--emerald-soft)" : "var(--text)" }}>
                      {fmtNum(log.cached_input_tokens)}
                    </div>
                  </div>

                  <div style={{ background: "var(--surface-raised)", padding: "10px 12px", borderRadius: 4, border: "1px solid var(--border)" }}>
                    <div style={{ fontSize: 10, color: "var(--text-muted)", textTransform: "uppercase", marginBottom: 2 }}>
                      Reasoning Tokens
                    </div>
                    <div style={{ fontSize: 15, fontWeight: 600, color: log.reasoning_tokens > 0 ? "var(--amber-soft)" : "var(--text)" }}>
                      {fmtNum(log.reasoning_tokens)}
                    </div>
                  </div>
                </div>

                <div
                  style={{
                    marginTop: 12,
                    padding: "10px 14px",
                    background: "var(--surface-high)",
                    borderRadius: 4,
                    border: "1px solid var(--border)",
                    display: "flex",
                    justifyContent: "space-between",
                    alignItems: "center",
                    fontFamily: "var(--font-mono)",
                  }}
                >
                  <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                    <span style={{ fontSize: 11, color: "var(--text-muted)", textTransform: "uppercase" }}>Total Tokens</span>
                    <span style={{ fontSize: 16, fontWeight: 700, color: "var(--primary)" }}>{fmtNum(log.total_tokens)}</span>
                  </div>
                  {cacheSavings > 0 && (
                    <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                      <span style={{ fontSize: 11, color: "var(--text-muted)", textTransform: "uppercase" }}>Cache Savings</span>
                      <span style={{ fontSize: 14, fontWeight: 700, color: "var(--emerald)" }}>{fmtMoney(cacheSavings)}</span>
                    </div>
                  )}
                </div>
              </div>

              {/* Financials & Unit Pricing */}
              <div
                style={{
                  background: "var(--surface-high)",
                  border: "1px solid var(--border)",
                  borderRadius: 6,
                  padding: "16px",
                  display: "flex",
                  flexDirection: "column",
                  gap: 12,
                }}
              >
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <span
                    style={{
                      fontSize: 11,
                      fontFamily: "var(--font-mono)",
                      fontWeight: 700,
                      color: "var(--text-muted)",
                      letterSpacing: "0.05em",
                      textTransform: "uppercase",
                    }}
                  >
                    Financials & Cost Audit
                  </span>
                  <span
                    className="mono"
                    style={{
                      fontSize: 10,
                      padding: "2px 6px",
                      borderRadius: 3,
                      background: log.pricing_fallback ? "rgba(245, 158, 11, 0.15)" : "rgba(34, 211, 238, 0.12)",
                      color: log.pricing_fallback ? "var(--amber-soft)" : "var(--primary)",
                      border: `1px solid ${log.pricing_fallback ? "rgba(245, 158, 11, 0.3)" : "rgba(34, 211, 238, 0.3)"}`,
                      textTransform: "uppercase",
                    }}
                  >
                    [{log.pricing_source || "database"}]
                  </span>
                </div>

                <div
                  style={{
                    display: "flex",
                    justifyContent: "space-between",
                    alignItems: "flex-end",
                    borderBottom: "1px solid var(--border)",
                    paddingBottom: 10,
                  }}
                >
                  <span style={{ fontSize: 14, color: "var(--text)" }}>Total Request Cost</span>
                  <span className="mono" style={{ fontSize: 20, fontWeight: 700, color: "var(--emerald)" }}>
                    {fmtMoney(log.cost)}
                  </span>
                </div>

                <div className="mono" style={{ display: "flex", justifyContent: "space-between", fontSize: 12, color: "var(--text-muted)" }}>
                  <span>Pricing Applied</span>
                  <div style={{ textAlign: "right" }}>
                    <div>${log.prompt_price_used.toFixed(2)}/1M (prompt)</div>
                    <div>${log.completion_price_used.toFixed(2)}/1M (completion)</div>
                  </div>
                </div>
              </div>

              {/* Error Details (if failed) */}
              {(log.status_code >= 400 || log.error_message) && (
                <div
                  style={{
                    background: "rgba(248, 113, 113, 0.08)",
                    border: "1px solid rgba(248, 113, 113, 0.3)",
                    borderRadius: 6,
                    padding: "14px 16px",
                  }}
                >
                  <div style={{ display: "flex", alignItems: "center", gap: 6, color: "var(--danger)", marginBottom: 8 }}>
                    <span className="material-symbols-outlined" style={{ fontSize: 18 }}>
                      error
                    </span>
                    <span className="mono" style={{ fontSize: 12, fontWeight: 700 }}>
                      HTTP {log.status_code}: {status?.text}
                    </span>
                  </div>
                  <div
                    className="mono"
                    style={{
                      fontSize: 12,
                      color: "var(--danger-soft)",
                      whiteSpace: "pre-wrap",
                      wordBreak: "break-word",
                      background: "rgba(0, 0, 0, 0.2)",
                      padding: "8px 10px",
                      borderRadius: 4,
                    }}
                  >
                    {log.error_message || "No error details recorded by upstream provider."}
                  </div>
                </div>
              )}

              {/* Zero-Leak Structural Tracing */}
              {log.request_shape && (
                <div
                  style={{
                    background: "var(--surface-high)",
                    border: "1px solid var(--border)",
                    borderRadius: 6,
                    padding: "16px",
                    display: "flex",
                    flexDirection: "column",
                    gap: 12,
                  }}
                >
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 8 }}>
                    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                      <span
                        style={{
                          fontSize: 11,
                          fontFamily: "var(--font-mono)",
                          fontWeight: 700,
                          color: "var(--text-muted)",
                          letterSpacing: "0.05em",
                          textTransform: "uppercase",
                        }}
                      >
                        Structural Shape &amp; Modality
                      </span>
                      <span
                        className="mono"
                        style={{
                          fontSize: 10,
                          padding: "2px 6px",
                          borderRadius: 3,
                          background:
                            log.request_shape.modality === "vision"
                              ? "rgba(34, 211, 238, 0.15)"
                              : log.request_shape.modality === "audio"
                              ? "rgba(245, 158, 11, 0.15)"
                              : log.request_shape.modality === "tools"
                              ? "rgba(168, 85, 247, 0.15)"
                              : "rgba(16, 185, 129, 0.15)",
                          color:
                            log.request_shape.modality === "vision"
                              ? "var(--primary)"
                              : log.request_shape.modality === "audio"
                              ? "var(--amber-soft)"
                              : log.request_shape.modality === "tools"
                              ? "var(--violet)"
                              : "var(--emerald-soft)",
                          border: "1px solid currentColor",
                          fontWeight: 700,
                          textTransform: "uppercase",
                        }}
                      >
                        [{log.request_shape.modality}]
                      </span>
                    </div>

                    <span
                      style={{
                        fontSize: 11,
                        color: "var(--emerald-soft)",
                        display: "flex",
                        alignItems: "center",
                        gap: 4,
                        fontFamily: "var(--font-mono)",
                      }}
                    >
                      <span className="material-symbols-outlined" style={{ fontSize: 14 }}>
                        shield
                      </span>
                      Zero PII Stored
                    </span>
                  </div>

                  {/* Structure Fingerprint Hash */}
                  <div
                    style={{
                      display: "flex",
                      justifyContent: "space-between",
                      alignItems: "center",
                      background: "rgba(0, 0, 0, 0.25)",
                      padding: "8px 10px",
                      borderRadius: 4,
                      border: "1px solid var(--border)",
                    }}
                  >
                    <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                      <span style={{ fontSize: 10, color: "var(--text-muted)", textTransform: "uppercase", fontFamily: "var(--font-mono)" }}>
                        Signature Hash
                      </span>
                      <span className="mono" style={{ fontSize: 12, color: "var(--primary)", fontWeight: 600 }}>
                        {log.request_shape.structure_hash || "—"}
                      </span>
                    </div>
                    {log.request_shape.structure_hash && (
                      <button
                        className="btn btn--ghost btn--sm"
                        style={{ padding: "1px 6px", height: "auto", fontSize: 10, fontFamily: "var(--font-mono)" }}
                        onClick={() => handleCopy(log.request_shape!.structure_hash, "Hash")}
                      >
                        {copied === "Hash" ? "Copied" : "Copy"}
                      </button>
                    )}
                  </div>

                  {/* Metrics grid */}
                  <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 8 }}>
                    <div style={{ background: "var(--surface-raised)", padding: "8px 10px", borderRadius: 4, border: "1px solid var(--border)" }}>
                      <div style={{ fontSize: 10, color: "var(--text-muted)", textTransform: "uppercase" }}>Messages</div>
                      <div className="mono" style={{ fontSize: 14, fontWeight: 600, color: "var(--text)", marginTop: 2 }}>
                        {log.request_shape.message_count}
                      </div>
                    </div>
                    <div style={{ background: "var(--surface-raised)", padding: "8px 10px", borderRadius: 4, border: "1px solid var(--border)" }}>
                      <div style={{ fontSize: 10, color: "var(--text-muted)", textTransform: "uppercase" }}>Char Count</div>
                      <div className="mono" style={{ fontSize: 14, fontWeight: 600, color: "var(--text)", marginTop: 2 }}>
                        {fmtNum(log.request_shape.char_count)}
                      </div>
                    </div>
                    <div style={{ background: "var(--surface-raised)", padding: "8px 10px", borderRadius: 4, border: "1px solid var(--border)" }}>
                      <div style={{ fontSize: 10, color: "var(--text-muted)", textTransform: "uppercase" }}>Word Count</div>
                      <div className="mono" style={{ fontSize: 14, fontWeight: 600, color: "var(--text)", marginTop: 2 }}>
                        {fmtNum(log.request_shape.word_count)}
                      </div>
                    </div>
                    <div style={{ background: "var(--surface-raised)", padding: "8px 10px", borderRadius: 4, border: "1px solid var(--border)" }}>
                      <div style={{ fontSize: 10, color: "var(--text-muted)", textTransform: "uppercase" }}>System Chars</div>
                      <div className="mono" style={{ fontSize: 14, fontWeight: 600, color: "var(--text)", marginTop: 2 }}>
                        {fmtNum(log.request_shape.system_prompt_chars || 0)}
                      </div>
                    </div>
                  </div>

                  {/* Roles list */}
                  {log.request_shape.roles && log.request_shape.roles.length > 0 && (
                    <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap", fontSize: 12 }}>
                      <span style={{ fontSize: 10, color: "var(--text-muted)", textTransform: "uppercase", fontFamily: "var(--font-mono)" }}>
                        Roles:
                      </span>
                      <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
                        {log.request_shape.roles.map((r) => (
                          <span
                            key={r}
                            className="mono"
                            style={{
                              fontSize: 11,
                              padding: "1px 6px",
                              borderRadius: 3,
                              background: "rgba(255, 255, 255, 0.06)",
                              border: "1px solid var(--border)",
                              color:
                                r === "system"
                                  ? "var(--amber-soft)"
                                  : r === "user"
                                  ? "var(--primary)"
                                  : r === "assistant"
                                  ? "var(--emerald-soft)"
                                  : "var(--violet)",
                            }}
                          >
                            {r}
                          </span>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Multimodal Details (Images & Audio) */}
                  {(log.request_shape.has_images || log.request_shape.has_audio) && (
                    <div style={{ display: "flex", flexDirection: "column", gap: 8, paddingTop: 6, borderTop: "1px solid var(--border)" }}>
                      {log.request_shape.has_images && (
                        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", fontSize: 12 }}>
                          <span style={{ display: "flex", alignItems: "center", gap: 6, color: "var(--text)" }}>
                            <span className="material-symbols-outlined" style={{ fontSize: 16, color: "var(--primary)" }}>
                              image
                            </span>
                            Vision Payload: <strong>{log.request_shape.image_count} image{log.request_shape.image_count > 1 ? "s" : ""}</strong>
                          </span>
                          <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                            <span className="mono" style={{ color: "var(--primary)", fontWeight: 600 }}>
                              {fmtBytes(log.request_shape.image_total_bytes)}
                            </span>
                            {log.request_shape.image_formats?.map((fmt) => (
                              <span key={fmt} className="mono" style={{ fontSize: 10, color: "var(--text-muted)" }}>
                                ({fmt})
                              </span>
                            ))}
                          </div>
                        </div>
                      )}

                      {log.request_shape.has_audio && (
                        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", fontSize: 12 }}>
                          <span style={{ display: "flex", alignItems: "center", gap: 6, color: "var(--text)" }}>
                            <span className="material-symbols-outlined" style={{ fontSize: 16, color: "var(--amber-soft)" }}>
                              mic
                            </span>
                            Audio Payload: <strong>{log.request_shape.audio_count} clip{log.request_shape.audio_count > 1 ? "s" : ""}</strong>
                          </span>
                          <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                            <span className="mono" style={{ color: "var(--amber-soft)", fontWeight: 600 }}>
                              {fmtBytes(log.request_shape.audio_total_bytes)}
                            </span>
                            {log.request_shape.audio_formats?.map((fmt) => (
                              <span key={fmt} className="mono" style={{ fontSize: 10, color: "var(--text-muted)" }}>
                                ({fmt})
                              </span>
                            ))}
                          </div>
                        </div>
                      )}
                    </div>
                  )}

                  {/* Tool Calling Details */}
                  {(log.request_shape.tools_count > 0 || log.request_shape.tool_calls_count > 0) && (
                    <div style={{ display: "flex", flexDirection: "column", gap: 8, paddingTop: 6, borderTop: "1px solid var(--border)", fontSize: 12 }}>
                      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                        <span style={{ display: "flex", alignItems: "center", gap: 6, color: "var(--text)" }}>
                          <span className="material-symbols-outlined" style={{ fontSize: 16, color: "var(--violet)" }}>
                            build
                          </span>
                          Tool Calling: <strong>{log.request_shape.tool_calls_count} call{log.request_shape.tool_calls_count > 1 ? "s" : ""}</strong>
                          <span style={{ fontSize: 11, color: "var(--text-muted)" }}>({log.request_shape.tools_count} offered)</span>
                        </span>
                      </div>

                      {log.request_shape.tool_names && log.request_shape.tool_names.length > 0 && (
                        <div style={{ display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
                          <span style={{ fontSize: 10, color: "var(--text-muted)", textTransform: "uppercase", fontFamily: "var(--font-mono)" }}>
                            Tools:
                          </span>
                          {log.request_shape.tool_names.map((name) => (
                            <span
                              key={name}
                              className="mono"
                              style={{
                                fontSize: 11,
                                padding: "1px 6px",
                                borderRadius: 3,
                                background: "rgba(168, 85, 247, 0.12)",
                                color: "var(--violet)",
                                border: "1px solid rgba(168, 85, 247, 0.3)",
                              }}
                            >
                              {name}
                            </span>
                          ))}
                        </div>
                      )}

                      {log.request_shape.tool_parameter_keys && log.request_shape.tool_parameter_keys.length > 0 && (
                        <div style={{ display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
                          <span style={{ fontSize: 10, color: "var(--text-muted)", textTransform: "uppercase", fontFamily: "var(--font-mono)" }}>
                            Param Keys:
                          </span>
                          {log.request_shape.tool_parameter_keys.map((k) => (
                            <span
                              key={k}
                              className="mono"
                              style={{
                                fontSize: 10,
                                padding: "1px 5px",
                                borderRadius: 3,
                                background: "var(--surface-raised)",
                                color: "var(--text-muted)",
                                border: "1px solid var(--border)",
                              }}
                            >
                              {k}
                            </span>
                          ))}
                        </div>
                      )}
                    </div>
                  )}

                  {/* Extended Thinking / Reasoning */}
                  {log.request_shape.has_reasoning && (
                    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", paddingTop: 6, borderTop: "1px solid var(--border)", fontSize: 12 }}>
                      <span style={{ display: "flex", alignItems: "center", gap: 6, color: "var(--text)" }}>
                        <span className="material-symbols-outlined" style={{ fontSize: 16, color: "var(--amber-soft)" }}>
                          psychology
                        </span>
                        Extended Thinking / Reasoning
                      </span>
                      <span className="mono" style={{ fontSize: 11, color: "var(--amber-soft)", fontWeight: 600 }}>
                        {log.request_shape.reasoning_effort || "active"}
                      </span>
                    </div>
                  )}

                  {/* Inference Hyperparameters & Execution Settings */}
                  {(log.request_shape.temperature !== undefined ||
                    log.request_shape.top_p !== undefined ||
                    log.request_shape.top_k !== undefined ||
                    log.request_shape.max_tokens !== undefined ||
                    log.request_shape.presence_penalty !== undefined ||
                    log.request_shape.frequency_penalty !== undefined ||
                    log.request_shape.seed !== undefined ||
                    (log.request_shape.stop_sequences && log.request_shape.stop_sequences.length > 0)) && (
                    <div style={{ display: "flex", flexDirection: "column", gap: 8, paddingTop: 6, borderTop: "1px solid var(--border)", fontSize: 12 }}>
                      <div style={{ display: "flex", alignItems: "center", gap: 6, color: "var(--text)" }}>
                        <span className="material-symbols-outlined" style={{ fontSize: 16, color: "var(--primary)" }}>
                          tune
                        </span>
                        <span style={{ fontSize: 10, color: "var(--text-muted)", textTransform: "uppercase", fontFamily: "var(--font-mono)" }}>
                          Hyperparameters &amp; Settings
                        </span>
                      </div>

                      <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
                        {log.request_shape.temperature !== undefined && (
                          <span
                            className="mono"
                            style={{
                              fontSize: 11,
                              padding: "2px 8px",
                              borderRadius: 4,
                              background: "rgba(34, 211, 238, 0.08)",
                              color: "var(--primary)",
                              border: "1px solid rgba(34, 211, 238, 0.2)",
                            }}
                          >
                            temperature: <strong>{log.request_shape.temperature}</strong>
                          </span>
                        )}
                        {log.request_shape.top_p !== undefined && (
                          <span
                            className="mono"
                            style={{
                              fontSize: 11,
                              padding: "2px 8px",
                              borderRadius: 4,
                              background: "rgba(34, 211, 238, 0.08)",
                              color: "var(--primary)",
                              border: "1px solid rgba(34, 211, 238, 0.2)",
                            }}
                          >
                            top_p: <strong>{log.request_shape.top_p}</strong>
                          </span>
                        )}
                        {log.request_shape.top_k !== undefined && (
                          <span
                            className="mono"
                            style={{
                              fontSize: 11,
                              padding: "2px 8px",
                              borderRadius: 4,
                              background: "rgba(168, 85, 247, 0.08)",
                              color: "var(--violet)",
                              border: "1px solid rgba(168, 85, 247, 0.2)",
                            }}
                          >
                            top_k: <strong>{log.request_shape.top_k}</strong>
                          </span>
                        )}
                        {log.request_shape.max_tokens !== undefined && (
                          <span
                            className="mono"
                            style={{
                              fontSize: 11,
                              padding: "2px 8px",
                              borderRadius: 4,
                              background: "rgba(16, 185, 129, 0.08)",
                              color: "var(--emerald-soft)",
                              border: "1px solid rgba(16, 185, 129, 0.2)",
                            }}
                          >
                            max_tokens: <strong>{fmtNum(log.request_shape.max_tokens)}</strong>
                          </span>
                        )}
                        {log.request_shape.seed !== undefined && (
                          <span
                            className="mono"
                            style={{
                              fontSize: 11,
                              padding: "2px 8px",
                              borderRadius: 4,
                              background: "var(--surface-raised)",
                              color: "var(--text-muted)",
                              border: "1px solid var(--border)",
                            }}
                          >
                            seed: <strong>{log.request_shape.seed}</strong>
                          </span>
                        )}
                        {log.request_shape.presence_penalty !== undefined && (
                          <span
                            className="mono"
                            style={{
                              fontSize: 11,
                              padding: "2px 8px",
                              borderRadius: 4,
                              background: "var(--surface-raised)",
                              color: "var(--text-muted)",
                              border: "1px solid var(--border)",
                            }}
                          >
                            presence_penalty: <strong>{log.request_shape.presence_penalty}</strong>
                          </span>
                        )}
                        {log.request_shape.frequency_penalty !== undefined && (
                          <span
                            className="mono"
                            style={{
                              fontSize: 11,
                              padding: "2px 8px",
                              borderRadius: 4,
                              background: "var(--surface-raised)",
                              color: "var(--text-muted)",
                              border: "1px solid var(--border)",
                            }}
                          >
                            frequency_penalty: <strong>{log.request_shape.frequency_penalty}</strong>
                          </span>
                        )}
                      </div>

                      {log.request_shape.stop_sequences && log.request_shape.stop_sequences.length > 0 && (
                        <div style={{ display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap", marginTop: 2 }}>
                          <span style={{ fontSize: 10, color: "var(--text-muted)", textTransform: "uppercase", fontFamily: "var(--font-mono)" }}>
                            Stop Sequences:
                          </span>
                          {log.request_shape.stop_sequences.map((s, idx) => (
                            <span
                              key={idx}
                              className="mono"
                              style={{
                                fontSize: 10,
                                padding: "1px 6px",
                                borderRadius: 3,
                                background: "var(--surface-raised)",
                                color: "var(--text)",
                                border: "1px solid var(--border)",
                              }}
                            >
                              {JSON.stringify(s)}
                            </span>
                          ))}
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )}

              {/* Request Metadata */}
              <div>
                <div
                  style={{
                    fontSize: 11,
                    fontFamily: "var(--font-mono)",
                    fontWeight: 700,
                    color: "var(--text-muted)",
                    letterSpacing: "0.05em",
                    textTransform: "uppercase",
                    marginBottom: 10,
                  }}
                >
                  Request Metadata
                </div>

                <ul
                  className="mono"
                  style={{
                    listStyle: "none",
                    padding: 0,
                    margin: 0,
                    fontSize: 12,
                    display: "flex",
                    flexDirection: "column",
                    gap: 8,
                  }}
                >
                  <li
                    style={{
                      display: "flex",
                      justifyContent: "space-between",
                      borderBottom: "1px solid var(--border)",
                      paddingBottom: 6,
                    }}
                  >
                    <span style={{ color: "var(--text-muted)" }}>User</span>
                    <span style={{ color: "var(--text)", textAlign: "right" }}>
                      {"user_email" in log && log.user_email ? (
                        <>
                          <strong>{log.user_email}</strong>{" "}
                          <span style={{ fontSize: 10, color: "var(--text-faint)" }}>({log.user_id.slice(0, 8)}…)</span>
                        </>
                      ) : (
                        log.user_id
                      )}
                    </span>
                  </li>

                  <li
                    style={{
                      display: "flex",
                      justifyContent: "space-between",
                      borderBottom: "1px solid var(--border)",
                      paddingBottom: 6,
                    }}
                  >
                    <span style={{ color: "var(--text-muted)" }}>API Key</span>
                    <span style={{ color: "var(--text)", textAlign: "right" }}>
                      {"key_name" in log && log.key_name ? (
                        <>
                          <strong>{log.key_name}</strong>{" "}
                          <span style={{ fontSize: 10, color: "var(--text-faint)" }}>({log.key_id.slice(0, 8)}…)</span>
                        </>
                      ) : (
                        log.key_id
                      )}
                    </span>
                  </li>

                  <li
                    style={{
                      display: "flex",
                      justifyContent: "space-between",
                      borderBottom: "1px solid var(--border)",
                      paddingBottom: 6,
                    }}
                  >
                    <span style={{ color: "var(--text-muted)" }}>Streaming</span>
                    <span style={{ color: isStreaming ? "var(--primary)" : "var(--text-muted)" }}>
                      {isStreaming ? "true" : "false"}
                    </span>
                  </li>

                  <li
                    style={{
                      display: "flex",
                      justifyContent: "space-between",
                      borderBottom: "1px solid var(--border)",
                      paddingBottom: 6,
                    }}
                  >
                    <span style={{ color: "var(--text-muted)" }}>Created At</span>
                    <span style={{ color: "var(--text)" }}>{fmtDate(log.created_at)}</span>
                  </li>
                </ul>
              </div>
            </>
          )}
        </div>

        {/* Drawer Actions */}
        <div className="drawer__foot">
          <button
            className="btn btn--ghost"
            style={{ flex: 1, display: "flex", alignItems: "center", justifyContent: "center", gap: 6 }}
            onClick={() => handleCopy(curlString, "cURL")}
            disabled={!log}
          >
            <span className="material-symbols-outlined" style={{ fontSize: 16 }}>
              content_copy
            </span>
            {copied === "cURL" ? "Copied!" : "Copy cURL"}
          </button>

          <button
            className="btn btn--primary"
            style={{ flex: 1, display: "flex", alignItems: "center", justifyContent: "center", gap: 6 }}
            onClick={() => handleCopy(jsonString, "JSON")}
            disabled={!log}
          >
            <span className="material-symbols-outlined" style={{ fontSize: 16 }}>
              data_object
            </span>
            {copied === "JSON" ? "Copied!" : "Copy JSON"}
          </button>
        </div>
      </aside>
    </div>
  );
}
