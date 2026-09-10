import { FormEvent, useEffect, useState } from "react";
import Modal from "./Modal";
import type { Plugin, PluginBinding, Tenant, ModelPricing, APIKey, User } from "../api/types";
import { api } from "../api/client";
import { useToast } from "./Toast";

interface Props {
  plugin?: Plugin | null;
  allPlugins: Plugin[];
  binding?: PluginBinding | null;
  onClose: () => void;
  onSuccess: () => void;
}

export default function PluginBindingModal({
  plugin: initialPlugin,
  allPlugins,
  binding,
  onClose,
  onSuccess,
}: Props) {
  const toast = useToast();

  const [selectedPluginName, setSelectedPluginName] = useState<string>(
    binding?.plugin_name || initialPlugin?.name || (allPlugins.length > 0 ? allPlugins[0].name : "")
  );

  const activePlugin = allPlugins.find((p) => p.name === selectedPluginName) || initialPlugin;

  const [scopeLevel, setScopeLevel] = useState<PluginBinding["scope_level"]>(
    binding?.scope_level || "global"
  );
  const [scopeProvider, setScopeProvider] = useState<string>(binding?.scope_provider || "openai");
  const [scopeModel, setScopeModel] = useState<string>(binding?.scope_model || "");
  const [scopeTenantId, setScopeTenantId] = useState<string>(binding?.scope_tenant_id || "");
  const [scopeRole, setScopeRole] = useState<string>(binding?.scope_role || "user");
  const [scopeUserId, setScopeUserId] = useState<string>(binding?.scope_user_id || "");
  const [scopeKeyId, setScopeKeyId] = useState<string>(binding?.scope_key_id || "");

  const [priority, setPriority] = useState<number>(binding?.priority ?? 100);
  const [isActive, setIsActive] = useState<boolean>(binding?.is_active ?? true);

  // Entities for scope dropdowns
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [models, setModels] = useState<ModelPricing[]>([]);
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [users, setUsers] = useState<User[]>([]);

  // Config fields
  const [rawConfig, setRawConfig] = useState<string>(
    binding ? JSON.stringify(binding.config, null, 2) : "{}"
  );
  const [useRawJson, setUseRawJson] = useState<boolean>(false);

  // Specific structured configs for known plugins
  // 1. rate-limiting
  const [rpm, setRpm] = useState<number>(binding?.config?.requests_per_minute ?? 60);
  const [tpm, setTpm] = useState<number>(binding?.config?.tokens_per_minute ?? 100000);
  const [burst, setBurst] = useState<number>(binding?.config?.burst_multiplier ?? 1.5);

  // 2. budget-control
  const [budgetLimit, setBudgetLimit] = useState<number>(binding?.config?.budget_limit ?? 100);
  const [budgetAction, setBudgetAction] = useState<string>(binding?.config?.action ?? "block");

  // 3. pii-masking
  const [maskEmail, setMaskEmail] = useState<boolean>(
    binding?.config?.patterns ? binding.config.patterns.includes("email") : true
  );
  const [maskPhone, setMaskPhone] = useState<boolean>(
    binding?.config?.patterns ? binding.config.patterns.includes("phone") : true
  );
  const [maskCard, setMaskCard] = useState<boolean>(
    binding?.config?.patterns ? binding.config.patterns.includes("credit_card") : true
  );
  const [maskNationalId, setMaskNationalId] = useState<boolean>(
    binding?.config?.patterns ? binding.config.patterns.includes("national_id") : true
  );
  const [maskApiKey, setMaskApiKey] = useState<boolean>(
    binding?.config?.patterns ? binding.config.patterns.includes("api_key") : true
  );
  const [replacement, setReplacement] = useState<string>(
    binding?.config?.replacement || "[REDACTED]"
  );

  // 4. prompt-guard
  const [checkInjection, setCheckInjection] = useState<boolean>(
    binding?.config?.check_injection ?? true
  );
  const [checkJailbreak, setCheckJailbreak] = useState<boolean>(
    binding?.config?.check_jailbreak ?? true
  );
  const [checkSystemLeak, setCheckSystemLeak] = useState<boolean>(
    binding?.config?.check_system_leak ?? true
  );
  const [bannedKeywordsText, setBannedKeywordsText] = useState<string>(
    binding?.config?.banned_keywords ? (binding.config.banned_keywords as string[]).join(", ") : ""
  );
  const [guardAction, setGuardAction] = useState<string>(
    binding?.config?.action ?? "block"
  );

  // 5. semantic-cache
  const [cacheTtl, setCacheTtl] = useState<number>(binding?.config?.ttl_seconds ?? 300);
  const [cacheHeader, setCacheHeader] = useState<boolean>(binding?.config?.cache_header ?? true);

  // 6. request-hedging
  const [hedgingDelay, setHedgingDelay] = useState<number>(binding?.config?.hedging_delay_ms ?? 300);
  const [secProvider, setSecProvider] = useState<string>(binding?.config?.secondary_provider ?? "");

  // 7. header-transform
  const [headerRows, setHeaderRows] = useState<Array<{ key: string; val: string }>>(() => {
    if (binding?.config?.set) {
      return Object.entries(binding.config.set).map(([key, val]) => ({
        key,
        val: String(val),
      }));
    }
    return [{ key: "", val: "" }];
  });
  const [removeHeadersText, setRemoveHeadersText] = useState<string>(
    binding?.config?.remove ? (binding.config.remove as string[]).join(", ") : ""
  );

  // 8. retry-circuit-breaker
  const [failThreshold, setFailThreshold] = useState<number>(binding?.config?.failure_threshold ?? 5);
  const [cooldownSec, setCooldownSec] = useState<number>(binding?.config?.cooldown_seconds ?? 30);

  // 9. langfuse-exporter
  const [langfuseHost, setLangfuseHost] = useState<string>(binding?.config?.host ?? "https://cloud.langfuse.com");
  const [langfusePubKey, setLangfusePubKey] = useState<string>(binding?.config?.public_key ?? "");
  const [langfuseSecKey, setLangfuseSecKey] = useState<string>(binding?.config?.secret_key ?? "");

  // 10. audit-trail
  const [auditDest, setAuditDest] = useState<string>(binding?.config?.destination ?? "log");
  const [auditWebhook, setAuditWebhook] = useState<string>(binding?.config?.webhook_url ?? "");

  // 11. prometheus-metrics
  const [promLatency, setPromLatency] = useState<boolean>(binding?.config?.record_latency ?? true);
  const [promTokens, setPromTokens] = useState<boolean>(binding?.config?.record_tokens ?? true);
  const [promStatusCodes, setPromStatusCodes] = useState<boolean>(binding?.config?.record_status_codes ?? true);
  const [promTenantLabel, setPromTenantLabel] = useState<boolean>(binding?.config?.include_tenant_label ?? true);
  const [promModelLabel, setPromModelLabel] = useState<boolean>(binding?.config?.include_model_label ?? true);
  const [promProviderLabel, setPromProviderLabel] = useState<boolean>(binding?.config?.include_provider_label ?? true);

  // 12. system-prompt-injector
  const [systemPromptText, setSystemPromptText] = useState<string>(binding?.config?.system_prompt ?? "");
  const [injectorMode, setInjectorMode] = useState<string>(binding?.config?.mode ?? "prepend");

  // 13. ip-restriction
  const [ipAllowlist, setIpAllowlist] = useState<string>(
    binding?.config?.allowlist ? (binding.config.allowlist as string[]).join("\n") : ""
  );
  const [ipDenylist, setIpDenylist] = useState<string>(
    binding?.config?.denylist ? (binding.config.denylist as string[]).join("\n") : ""
  );

  // 14. canary-traffic-splitter
  const [canaryModel, setCanaryModel] = useState<string>(binding?.config?.canary_model ?? "");
  const [canaryPercentage, setCanaryPercentage] = useState<number>(binding?.config?.canary_percentage ?? 10);
  const [canaryProvider, setCanaryProvider] = useState<string>(binding?.config?.canary_provider ?? "");

  // 15. prompt-compression
  const [collapseSpaces, setCollapseSpaces] = useState<boolean>(binding?.config?.collapse_spaces ?? true);
  const [collapseNewlines, setCollapseNewlines] = useState<boolean>(binding?.config?.collapse_newlines ?? true);
  const [trimWhitespace, setTrimWhitespace] = useState<boolean>(binding?.config?.trim_whitespace ?? true);

  // 16. response-watermarking
  const [watermarkId, setWatermarkId] = useState<string>(binding?.config?.watermark_id ?? "llm-gw-verified");
  const [watermarkHeader, setWatermarkHeader] = useState<string>(binding?.config?.header_name ?? "X-Gateway-Watermark");
  const [appendFooter, setAppendFooter] = useState<boolean>(binding?.config?.append_footer ?? false);
  const [footerText, setFooterText] = useState<string>(binding?.config?.footer_text ?? "\n\n---\n*Verified by LLM Gateway*");

  // 17. webhook-notifier
  const [alertWebhookUrl, setAlertWebhookUrl] = useState<string>(binding?.config?.webhook_url ?? "");
  const [notifyErrors, setNotifyErrors] = useState<boolean>(binding?.config?.notify_on_errors ?? true);
  const [notifySlow, setNotifySlow] = useState<boolean>(binding?.config?.notify_on_slow ?? true);
  const [slowThreshold, setSlowThreshold] = useState<number>(binding?.config?.slow_threshold_seconds ?? 5.0);
  const [minTokensThreshold, setMinTokensThreshold] = useState<number>(binding?.config?.min_tokens_threshold ?? 0);

  // 18. content-moderation
  const [modBlockHate, setModBlockHate] = useState<boolean>(binding?.config?.block_hate ?? true);
  const [modBlockViolence, setModBlockViolence] = useState<boolean>(binding?.config?.block_violence ?? true);
  const [modBlockSelfHarm, setModBlockSelfHarm] = useState<boolean>(binding?.config?.block_self_harm ?? true);
  const [modAction, setModAction] = useState<string>(binding?.config?.action ?? "block");
  const [customBannedWords, setCustomBannedWords] = useState<string>(
    binding?.config?.custom_banned_words ? (binding.config.custom_banned_words as string[]).join(", ") : ""
  );

  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    api.listTenants().then(setTenants).catch(() => {});
    api.listModels().then(setModels).catch(() => {});
    api.listKeys().then(setKeys).catch(() => {});
    api.listUsers().then(setUsers).catch(() => {});
  }, []);

  const buildConfigObject = (): Record<string, any> => {
    if (useRawJson) {
      return JSON.parse(rawConfig);
    }

    if (selectedPluginName === "rate-limiting") {
      return {
        requests_per_minute: Number(rpm),
        tokens_per_minute: Number(tpm),
        burst_multiplier: Number(burst),
      };
    }

    if (selectedPluginName === "budget-control") {
      return {
        budget_limit: Number(budgetLimit),
        action: budgetAction,
      };
    }

    if (selectedPluginName === "pii-masking") {
      const patterns: string[] = [];
      if (maskEmail) patterns.push("email");
      if (maskPhone) patterns.push("phone");
      if (maskCard) patterns.push("credit_card");
      if (maskNationalId) patterns.push("national_id");
      if (maskApiKey) patterns.push("api_key");
      return {
        patterns,
        replacement: replacement || "[REDACTED]",
      };
    }

    if (selectedPluginName === "prompt-guard") {
      const kws = bannedKeywordsText
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean);
      return {
        check_injection: checkInjection,
        check_jailbreak: checkJailbreak,
        check_system_leak: checkSystemLeak,
        banned_keywords: kws,
        action: guardAction,
      };
    }

    if (selectedPluginName === "semantic-cache") {
      return {
        ttl_seconds: Number(cacheTtl),
        cache_header: cacheHeader,
      };
    }

    if (selectedPluginName === "request-hedging") {
      return {
        hedging_delay_ms: Number(hedgingDelay),
        secondary_provider: secProvider,
      };
    }

    if (selectedPluginName === "header-transform") {
      const setMap: Record<string, string> = {};
      headerRows.forEach((r) => {
        if (r.key.trim()) {
          setMap[r.key.trim()] = r.val;
        }
      });
      const removeList = removeHeadersText
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean);
      return {
        set: setMap,
        remove: removeList,
      };
    }

    if (selectedPluginName === "retry-circuit-breaker") {
      return {
        failure_threshold: Number(failThreshold),
        cooldown_seconds: Number(cooldownSec),
      };
    }

    if (selectedPluginName === "langfuse-exporter") {
      return {
        host: langfuseHost,
        public_key: langfusePubKey,
        secret_key: langfuseSecKey,
      };
    }

    if (selectedPluginName === "audit-trail") {
      return {
        destination: auditDest,
        webhook_url: auditWebhook,
      };
    }

    if (selectedPluginName === "prometheus-metrics") {
      return {
        record_latency: promLatency,
        record_tokens: promTokens,
        record_status_codes: promStatusCodes,
        include_tenant_label: promTenantLabel,
        include_model_label: promModelLabel,
        include_provider_label: promProviderLabel,
      };
    }

    if (selectedPluginName === "system-prompt-injector") {
      return {
        system_prompt: systemPromptText,
        mode: injectorMode,
      };
    }

    if (selectedPluginName === "ip-restriction") {
      return {
        allowlist: ipAllowlist.split(/[\n,]/).map((s) => s.trim()).filter(Boolean),
        denylist: ipDenylist.split(/[\n,]/).map((s) => s.trim()).filter(Boolean),
      };
    }

    if (selectedPluginName === "canary-traffic-splitter") {
      return {
        canary_model: canaryModel,
        canary_percentage: Number(canaryPercentage),
        canary_provider: canaryProvider || undefined,
      };
    }

    if (selectedPluginName === "prompt-compression") {
      return {
        collapse_spaces: collapseSpaces,
        collapse_newlines: collapseNewlines,
        trim_whitespace: trimWhitespace,
      };
    }

    if (selectedPluginName === "response-watermarking") {
      return {
        watermark_id: watermarkId,
        header_name: watermarkHeader,
        append_footer: appendFooter,
        footer_text: footerText,
      };
    }

    if (selectedPluginName === "webhook-notifier") {
      return {
        webhook_url: alertWebhookUrl,
        notify_on_errors: notifyErrors,
        notify_on_slow: notifySlow,
        slow_threshold_seconds: Number(slowThreshold),
        min_tokens_threshold: Number(minTokensThreshold),
      };
    }

    if (selectedPluginName === "content-moderation") {
      return {
        block_hate: modBlockHate,
        block_violence: modBlockViolence,
        block_self_harm: modBlockSelfHarm,
        action: modAction,
        custom_banned_words: customBannedWords.split(",").map((s) => s.trim()).filter(Boolean),
      };
    }

    try {
      return JSON.parse(rawConfig);
    } catch {
      return {};
    }
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setSubmitting(true);

    try {
      const configObj = buildConfigObject();

      if (binding) {
        await api.updatePluginBinding(binding.id, {
          config: configObj,
          priority: Number(priority),
          is_active: isActive,
        });
        toast.success("Plugin binding updated successfully");
      } else {
        await api.createPluginBinding({
          plugin_name: selectedPluginName,
          scope_level: scopeLevel,
          scope_provider: scopeLevel === "provider" ? scopeProvider : undefined,
          scope_model: scopeLevel === "model" ? scopeModel : undefined,
          scope_tenant_id: scopeLevel === "tenant" && scopeTenantId ? scopeTenantId : undefined,
          scope_role: scopeLevel === "role" ? scopeRole : undefined,
          scope_user_id: scopeLevel === "user" && scopeUserId ? scopeUserId : undefined,
          scope_key_id: scopeLevel === "api_key" && scopeKeyId ? scopeKeyId : undefined,
          config: configObj,
          priority: Number(priority),
          is_active: isActive,
        });
        toast.success("Plugin bound successfully");
      }
      onSuccess();
    } catch (err: any) {
      toast.error(err.message || "Failed to save binding");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      title={binding ? `Edit Binding: ${binding.plugin_display_name || binding.plugin_name}` : "Bind Plugin to Scope"}
      onClose={onClose}
      maxWidth={680}
    >
      <form onSubmit={handleSubmit} className="stack stack--gap-lg">
        {/* Plugin Selector */}
        {!binding && (
          <div className="field">
            <label className="field__label">Plugin</label>
            <select
              className="select"
              value={selectedPluginName}
              onChange={(e) => setSelectedPluginName(e.target.value)}
              required
            >
              {allPlugins.map((p) => (
                <option key={p.name} value={p.name}>
                  {p.display_name} ({p.name})
                </option>
              ))}
            </select>
            {activePlugin && (
              <span className="field__help">{activePlugin.description}</span>
            )}
          </div>
        )}

        {/* Scope Level */}
        {!binding && (
          <div className="field">
            <label className="field__label">Scope Hierarchy Level</label>
            <select
              className="select"
              value={scopeLevel}
              onChange={(e) => setScopeLevel(e.target.value as any)}
            >
              <option value="global">1. Global (Applies to entire gateway)</option>
              <option value="provider">2. Provider (e.g. OpenAI, Anthropic, Gemini)</option>
              <option value="model">3. Model (e.g. gpt-4o, claude-3-5-sonnet)</option>
              <option value="tenant">4. Tenant / Organization</option>
              <option value="role">5. User Role (e.g. admin, user)</option>
              <option value="user">6. Specific User</option>
              <option value="api_key">7. Specific API Key</option>
            </select>
            <span className="field__help">
              Priority inheritance: API Key &gt; User &gt; Role &gt; Tenant &gt; Model &gt; Provider &gt; Global
            </span>
          </div>
        )}

        {/* Scope Target Selectors */}
        {!binding && scopeLevel === "provider" && (
          <div className="field">
            <label className="field__label">Provider</label>
            <select
              className="select"
              value={scopeProvider}
              onChange={(e) => setScopeProvider(e.target.value)}
            >
              <option value="openai">OpenAI</option>
              <option value="anthropic">Anthropic</option>
              <option value="gemini">Google Gemini</option>
            </select>
          </div>
        )}

        {!binding && scopeLevel === "model" && (
          <div className="field">
            <label className="field__label">Model Name</label>
            <input
              type="text"
              className="input"
              placeholder="e.g. gpt-4o or claude-3-5-sonnet"
              value={scopeModel}
              onChange={(e) => setScopeModel(e.target.value)}
              list="known-models"
              required
            />
            <datalist id="known-models">
              {models.map((m) => (
                <option key={m.id} value={m.model_name} />
              ))}
            </datalist>
          </div>
        )}

        {!binding && scopeLevel === "tenant" && (
          <div className="field">
            <label className="field__label">Tenant</label>
            <select
              className="select"
              value={scopeTenantId}
              onChange={(e) => setScopeTenantId(e.target.value)}
              required
            >
              <option value="">-- Select Tenant --</option>
              {tenants.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.name} ({t.type})
                </option>
              ))}
            </select>
          </div>
        )}

        {!binding && scopeLevel === "role" && (
          <div className="field">
            <label className="field__label">Role</label>
            <select
              className="select"
              value={scopeRole}
              onChange={(e) => setScopeRole(e.target.value)}
            >
              <option value="user">User</option>
              <option value="admin">Admin</option>
            </select>
          </div>
        )}

        {!binding && scopeLevel === "user" && (
          <div className="field">
            <label className="field__label">User</label>
            <select
              className="select"
              value={scopeUserId}
              onChange={(e) => setScopeUserId(e.target.value)}
              required
            >
              <option value="">-- Select User --</option>
              {users.map((u) => (
                <option key={u.id} value={u.id}>
                  {u.email} ({u.name || u.role})
                </option>
              ))}
            </select>
          </div>
        )}

        {!binding && scopeLevel === "api_key" && (
          <div className="field">
            <label className="field__label">API Key</label>
            <select
              className="select"
              value={scopeKeyId}
              onChange={(e) => setScopeKeyId(e.target.value)}
              required
            >
              <option value="">-- Select API Key --</option>
              {keys.map((k) => (
                <option key={k.id} value={k.id}>
                  {k.name} ({k.key_prefix}...)
                </option>
              ))}
            </select>
          </div>
        )}

        {/* Plugin Specific Config Header */}
        <div style={{ borderTop: "1px solid var(--border-color, #e2e8f0)", paddingTop: "1rem" }}>
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "0.75rem" }}>
            <span style={{ fontWeight: 600 }}>Configuration</span>
            <button
              type="button"
              className="btn btn--sm btn--ghost"
              onClick={() => setUseRawJson(!useRawJson)}
            >
              {useRawJson ? "Visual Form" : "JSON Editor"}
            </button>
          </div>

          {useRawJson ? (
            <div className="field">
              <textarea
                className="input"
                style={{ fontFamily: "monospace", minHeight: 140 }}
                value={rawConfig}
                onChange={(e) => setRawConfig(e.target.value)}
              />
            </div>
          ) : (
            <>
              {/* 1. rate-limiting UI */}
              {selectedPluginName === "rate-limiting" && (
                <div className="stack stack--gap-md">
                  <div className="field">
                    <label className="field__label">Requests Per Minute (RPM)</label>
                    <input
                      type="number"
                      className="input"
                      value={rpm}
                      min={0}
                      onChange={(e) => setRpm(Number(e.target.value))}
                      required
                    />
                    <span className="field__help">Maximum requests allowed per 60 seconds (0 = unlimited).</span>
                  </div>
                  <div className="field">
                    <label className="field__label">Tokens Per Minute (TPM)</label>
                    <input
                      type="number"
                      className="input"
                      value={tpm}
                      min={0}
                      onChange={(e) => setTpm(Number(e.target.value))}
                    />
                  </div>
                  <div className="field">
                    <label className="field__label">Burst Multiplier</label>
                    <input
                      type="number"
                      step="0.1"
                      className="input"
                      value={burst}
                      min={1.0}
                      onChange={(e) => setBurst(Number(e.target.value))}
                    />
                  </div>
                </div>
              )}

              {/* 2. budget-control UI */}
              {selectedPluginName === "budget-control" && (
                <div className="stack stack--gap-md">
                  <div className="field">
                    <label className="field__label">Budget Limit ($ USD)</label>
                    <input
                      type="number"
                      step="0.01"
                      className="input"
                      value={budgetLimit}
                      min={0}
                      onChange={(e) => setBudgetLimit(Number(e.target.value))}
                      required
                    />
                    <span className="field__help">Maximum spend allowed for this scope in USD.</span>
                  </div>
                  <div className="field">
                    <label className="field__label">Action When Exceeded</label>
                    <select
                      className="select"
                      value={budgetAction}
                      onChange={(e) => setBudgetAction(e.target.value)}
                    >
                      <option value="block">Block Request (402 Payment Required)</option>
                      <option value="warn">Warn Only (Emit Header X-Budget-Warning)</option>
                    </select>
                  </div>
                </div>
              )}

              {/* 3. pii-masking UI */}
              {selectedPluginName === "pii-masking" && (
                <div className="stack stack--gap-md">
                  <div className="field">
                    <label className="field__label">Entities to Redact</label>
                    <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(140px, 1fr))", gap: "0.75rem", marginTop: "0.25rem" }}>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.4rem" }}>
                        <input
                          type="checkbox"
                          checked={maskEmail}
                          onChange={(e) => setMaskEmail(e.target.checked)}
                        />
                        Email Addresses
                      </label>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.4rem" }}>
                        <input
                          type="checkbox"
                          checked={maskPhone}
                          onChange={(e) => setMaskPhone(e.target.checked)}
                        />
                        Phone Numbers
                      </label>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.4rem" }}>
                        <input
                          type="checkbox"
                          checked={maskCard}
                          onChange={(e) => setMaskCard(e.target.checked)}
                        />
                        Credit Cards
                      </label>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.4rem" }}>
                        <input
                          type="checkbox"
                          checked={maskNationalId}
                          onChange={(e) => setMaskNationalId(e.target.checked)}
                        />
                        CCCD / National ID
                      </label>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.4rem" }}>
                        <input
                          type="checkbox"
                          checked={maskApiKey}
                          onChange={(e) => setMaskApiKey(e.target.checked)}
                        />
                        API Keys &amp; Secrets
                      </label>
                    </div>
                  </div>
                  <div className="field">
                    <label className="field__label">Replacement String</label>
                    <input
                      type="text"
                      className="input"
                      value={replacement}
                      onChange={(e) => setReplacement(e.target.value)}
                      placeholder="[REDACTED]"
                    />
                  </div>
                </div>
              )}

              {/* 4. prompt-guard UI */}
              {selectedPluginName === "prompt-guard" && (
                <div className="stack stack--gap-md">
                  <div className="field">
                    <label className="field__label">Guardrail Protections</label>
                    <div style={{ display: "flex", flexDirection: "column", gap: "0.5rem", marginTop: "0.25rem" }}>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.4rem" }}>
                        <input
                          type="checkbox"
                          checked={checkInjection}
                          onChange={(e) => setCheckInjection(e.target.checked)}
                        />
                        Block Prompt Injection &amp; Instruction Overrides
                      </label>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.4rem" }}>
                        <input
                          type="checkbox"
                          checked={checkJailbreak}
                          onChange={(e) => setCheckJailbreak(e.target.checked)}
                        />
                        Block Jailbreak Exploits (DAN mode, unfiltered roleplay)
                      </label>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.4rem" }}>
                        <input
                          type="checkbox"
                          checked={checkSystemLeak}
                          onChange={(e) => setCheckSystemLeak(e.target.checked)}
                        />
                        Detect System Prompt Leakage Attempts
                      </label>
                    </div>
                  </div>
                  <div className="field">
                    <label className="field__label">Custom Banned Keywords (comma-separated)</label>
                    <input
                      type="text"
                      className="input"
                      value={bannedKeywordsText}
                      onChange={(e) => setBannedKeywordsText(e.target.value)}
                      placeholder="e.g. bypass, hack, exploit"
                    />
                  </div>
                  <div className="field">
                    <label className="field__label">Enforcement Action</label>
                    <select
                      className="select"
                      value={guardAction}
                      onChange={(e) => setGuardAction(e.target.value)}
                    >
                      <option value="block">Block Request (400 Bad Request)</option>
                      <option value="log_only">Log &amp; Warn Only (X-Prompt-Guard-Warning)</option>
                    </select>
                  </div>
                </div>
              )}

              {/* 5. semantic-cache UI */}
              {selectedPluginName === "semantic-cache" && (
                <div className="stack stack--gap-md">
                  <div className="field">
                    <label className="field__label">Cache TTL (Seconds)</label>
                    <input
                      type="number"
                      className="input"
                      value={cacheTtl}
                      min={0}
                      onChange={(e) => setCacheTtl(Number(e.target.value))}
                    />
                    <span className="field__help">Duration to store responses in Redis cache (default 300s = 5m).</span>
                  </div>
                  <div className="field">
                    <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
                      <input
                        type="checkbox"
                        checked={cacheHeader}
                        onChange={(e) => setCacheHeader(e.target.checked)}
                      />
                      <span>Emit X-Cache: HIT / MISS Response Header</span>
                    </label>
                  </div>
                </div>
              )}

              {/* 6. request-hedging UI */}
              {selectedPluginName === "request-hedging" && (
                <div className="stack stack--gap-md">
                  <div className="field">
                    <label className="field__label">Hedging Delay (Milliseconds)</label>
                    <input
                      type="number"
                      className="input"
                      value={hedgingDelay}
                      min={50}
                      onChange={(e) => setHedgingDelay(Number(e.target.value))}
                    />
                    <span className="field__help">Wait time before triggering secondary dual-dispatch.</span>
                  </div>
                  <div className="field">
                    <label className="field__label">Secondary Provider Alias (Optional)</label>
                    <input
                      type="text"
                      className="input"
                      value={secProvider}
                      onChange={(e) => setSecProvider(e.target.value)}
                      placeholder="e.g. anthropic, gemini"
                    />
                  </div>
                </div>
              )}

              {/* 7. header-transform UI */}
              {selectedPluginName === "header-transform" && (
                <div className="stack stack--gap-md">
                  <div className="field">
                    <label className="field__label">Set Upstream Headers</label>
                    {headerRows.map((row, idx) => (
                      <div key={idx} style={{ display: "flex", gap: "0.5rem", marginBottom: "0.5rem" }}>
                        <input
                          type="text"
                          className="input"
                          placeholder="Header Name (e.g. X-Tenant-App)"
                          value={row.key}
                          onChange={(e) => {
                            const copy = [...headerRows];
                            copy[idx].key = e.target.value;
                            setHeaderRows(copy);
                          }}
                        />
                        <input
                          type="text"
                          className="input"
                          placeholder="Header Value"
                          value={row.val}
                          onChange={(e) => {
                            const copy = [...headerRows];
                            copy[idx].val = e.target.value;
                            setHeaderRows(copy);
                          }}
                        />
                        <button
                          type="button"
                          className="btn btn--sm btn--danger"
                          onClick={() => {
                            setHeaderRows(headerRows.filter((_, i) => i !== idx));
                          }}
                        >
                          ✕
                        </button>
                      </div>
                    ))}
                    <button
                      type="button"
                      className="btn btn--sm btn--outline"
                      onClick={() => setHeaderRows([...headerRows, { key: "", val: "" }])}
                    >
                      + Add Header
                    </button>
                  </div>
                  <div className="field">
                    <label className="field__label">Remove Headers (comma-separated)</label>
                    <input
                      type="text"
                      className="input"
                      placeholder="e.g. X-Forwarded-For, X-Powered-By"
                      value={removeHeadersText}
                      onChange={(e) => setRemoveHeadersText(e.target.value)}
                    />
                  </div>
                </div>
              )}

              {/* 8. retry-circuit-breaker UI */}
              {selectedPluginName === "retry-circuit-breaker" && (
                <div className="stack stack--gap-md">
                  <div className="field">
                    <label className="field__label">Failure Threshold</label>
                    <input
                      type="number"
                      className="input"
                      value={failThreshold}
                      min={1}
                      onChange={(e) => setFailThreshold(Number(e.target.value))}
                    />
                    <span className="field__help">Consecutive failures before opening circuit breaker.</span>
                  </div>
                  <div className="field">
                    <label className="field__label">Cooldown Duration (Seconds)</label>
                    <input
                      type="number"
                      className="input"
                      value={cooldownSec}
                      min={5}
                      onChange={(e) => setCooldownSec(Number(e.target.value))}
                    />
                    <span className="field__help">Duration circuit stays OPEN before half-open test.</span>
                  </div>
                </div>
              )}

              {/* 9. langfuse-exporter UI */}
              {selectedPluginName === "langfuse-exporter" && (
                <div className="stack stack--gap-md">
                  <div className="field">
                    <label className="field__label">Langfuse Host URL</label>
                    <input
                      type="text"
                      className="input"
                      value={langfuseHost}
                      onChange={(e) => setLangfuseHost(e.target.value)}
                      placeholder="https://cloud.langfuse.com"
                    />
                  </div>
                  <div className="field">
                    <label className="field__label">Public Key</label>
                    <input
                      type="text"
                      className="input"
                      value={langfusePubKey}
                      onChange={(e) => setLangfusePubKey(e.target.value)}
                      placeholder="pk-lf-..."
                      required
                    />
                  </div>
                  <div className="field">
                    <label className="field__label">Secret Key</label>
                    <input
                      type="password"
                      className="input"
                      value={langfuseSecKey}
                      onChange={(e) => setLangfuseSecKey(e.target.value)}
                      placeholder="sk-lf-..."
                      required
                    />
                  </div>
                </div>
              )}

              {/* 10. audit-trail UI */}
              {selectedPluginName === "audit-trail" && (
                <div className="stack stack--gap-md">
                  <div className="field">
                    <label className="field__label">Audit Destination</label>
                    <select
                      className="select"
                      value={auditDest}
                      onChange={(e) => setAuditDest(e.target.value)}
                    >
                      <option value="log">Structured Log Only</option>
                      <option value="webhook">External Webhook Endpoint</option>
                    </select>
                  </div>
                  {auditDest === "webhook" && (
                    <div className="field">
                      <label className="field__label">Webhook URL</label>
                      <input
                        type="url"
                        className="input"
                        value={auditWebhook}
                        onChange={(e) => setAuditWebhook(e.target.value)}
                        placeholder="https://siem.corp.internal/v1/audit"
                        required
                      />
                    </div>
                  )}
                </div>
              )}

              {/* 11. prometheus-metrics UI */}
              {selectedPluginName === "prometheus-metrics" && (
                <div className="stack stack--gap-md">
                  <div className="field">
                    <label className="field__label">Metrics Collection</label>
                    <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))", gap: "0.75rem", marginTop: "0.25rem" }}>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.4rem" }}>
                        <input
                          type="checkbox"
                          checked={promStatusCodes}
                          onChange={(e) => setPromStatusCodes(e.target.checked)}
                        />
                        Record Status Codes
                      </label>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.4rem" }}>
                        <input
                          type="checkbox"
                          checked={promLatency}
                          onChange={(e) => setPromLatency(e.target.checked)}
                        />
                        Record Latency Histogram
                      </label>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.4rem" }}>
                        <input
                          type="checkbox"
                          checked={promTokens}
                          onChange={(e) => setPromTokens(e.target.checked)}
                        />
                        Record Token Usage
                      </label>
                    </div>
                  </div>
                  <div className="field">
                    <label className="field__label">Label Cardinality (Prometheus Dimensions)</label>
                    <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))", gap: "0.75rem", marginTop: "0.25rem" }}>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.4rem" }}>
                        <input
                          type="checkbox"
                          checked={promTenantLabel}
                          onChange={(e) => setPromTenantLabel(e.target.checked)}
                        />
                        Include Tenant Label
                      </label>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.4rem" }}>
                        <input
                          type="checkbox"
                          checked={promModelLabel}
                          onChange={(e) => setPromModelLabel(e.target.checked)}
                        />
                        Include Model Label
                      </label>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.4rem" }}>
                        <input
                          type="checkbox"
                          checked={promProviderLabel}
                          onChange={(e) => setPromProviderLabel(e.target.checked)}
                        />
                        Include Provider Label
                      </label>
                    </div>
                    <span className="field__help">Disable labels for public API keys to avoid high TSDB cardinality.</span>
                  </div>
                </div>
              )}

              {/* 12. system-prompt-injector UI */}
              {selectedPluginName === "system-prompt-injector" && (
                <div className="stack stack--gap-md">
                  <div className="field">
                    <label className="field__label">Injection Mode</label>
                    <select
                      className="select"
                      value={injectorMode}
                      onChange={(e) => setInjectorMode(e.target.value)}
                    >
                      <option value="prepend">Prepend (Add before user system instructions)</option>
                      <option value="append">Append (Add after user system instructions)</option>
                      <option value="override">Override (Completely replace system instructions)</option>
                    </select>
                  </div>
                  <div className="field">
                    <label className="field__label">System Prompt Content</label>
                    <textarea
                      className="textarea"
                      rows={4}
                      value={systemPromptText}
                      onChange={(e) => setSystemPromptText(e.target.value)}
                      placeholder="Enter company policies, behavioral constraints, or system prompt guidelines..."
                      required
                    />
                  </div>
                </div>
              )}

              {/* 13. ip-restriction UI */}
              {selectedPluginName === "ip-restriction" && (
                <div className="stack stack--gap-md">
                  <div className="field">
                    <label className="field__label">IP / CIDR Allowlist (One per line)</label>
                    <textarea
                      className="textarea"
                      rows={3}
                      value={ipAllowlist}
                      onChange={(e) => setIpAllowlist(e.target.value)}
                      placeholder="192.168.1.0/24&#10;10.0.0.50"
                    />
                    <span className="field__help">If specified, only requests from these IPs/subnets are allowed.</span>
                  </div>
                  <div className="field">
                    <label className="field__label">IP / CIDR Denylist (One per line)</label>
                    <textarea
                      className="textarea"
                      rows={3}
                      value={ipDenylist}
                      onChange={(e) => setIpDenylist(e.target.value)}
                      placeholder="203.0.113.0/24&#10;198.51.100.25"
                    />
                    <span className="field__help">Requests matching these IPs will be blocked immediately (403 Forbidden).</span>
                  </div>
                </div>
              )}

              {/* 14. canary-traffic-splitter UI */}
              {selectedPluginName === "canary-traffic-splitter" && (
                <div className="stack stack--gap-md">
                  <div className="field">
                    <label className="field__label">Canary Target Model</label>
                    <input
                      type="text"
                      className="input"
                      value={canaryModel}
                      onChange={(e) => setCanaryModel(e.target.value)}
                      placeholder="claude-3-5-sonnet or gpt-4o-mini"
                      required
                    />
                  </div>
                  <div className="field">
                    <label className="field__label">Canary Traffic Split: {canaryPercentage}%</label>
                    <input
                      type="range"
                      min={1}
                      max={100}
                      className="range"
                      value={canaryPercentage}
                      onChange={(e) => setCanaryPercentage(Number(e.target.value))}
                    />
                    <span className="field__help">{canaryPercentage}% of traffic is routed to the canary model, {100 - canaryPercentage}% to primary.</span>
                  </div>
                  <div className="field">
                    <label className="field__label">Canary Upstream Provider (Optional)</label>
                    <input
                      type="text"
                      className="input"
                      value={canaryProvider}
                      onChange={(e) => setCanaryProvider(e.target.value)}
                      placeholder="anthropic or custom-deepseek"
                    />
                  </div>
                </div>
              )}

              {/* 15. prompt-compression UI */}
              {selectedPluginName === "prompt-compression" && (
                <div className="stack stack--gap-md">
                  <div className="field">
                    <label className="field__label">Compression Options</label>
                    <div style={{ display: "flex", flexDirection: "column", gap: "0.5rem", marginTop: "0.25rem" }}>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
                        <input
                          type="checkbox"
                          checked={collapseSpaces}
                          onChange={(e) => setCollapseSpaces(e.target.checked)}
                        />
                        Collapse multiple consecutive spaces/tabs into a single space
                      </label>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
                        <input
                          type="checkbox"
                          checked={collapseNewlines}
                          onChange={(e) => setCollapseNewlines(e.target.checked)}
                        />
                        Collapse 3+ consecutive newlines into 2 (double newline)
                      </label>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
                        <input
                          type="checkbox"
                          checked={trimWhitespace}
                          onChange={(e) => setTrimWhitespace(e.target.checked)}
                        />
                        Trim leading &amp; trailing whitespace
                      </label>
                    </div>
                  </div>
                </div>
              )}

              {/* 16. response-watermarking UI */}
              {selectedPluginName === "response-watermarking" && (
                <div className="stack stack--gap-md">
                  <div className="field">
                    <label className="field__label">Watermark ID / Signature</label>
                    <input
                      type="text"
                      className="input"
                      value={watermarkId}
                      onChange={(e) => setWatermarkId(e.target.value)}
                      placeholder="corp-gateway-v1"
                      required
                    />
                  </div>
                  <div className="field">
                    <label className="field__label">HTTP Verification Header Name</label>
                    <input
                      type="text"
                      className="input"
                      value={watermarkHeader}
                      onChange={(e) => setWatermarkHeader(e.target.value)}
                      placeholder="X-Gateway-Watermark"
                    />
                  </div>
                  <div className="field">
                    <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
                      <input
                        type="checkbox"
                        checked={appendFooter}
                        onChange={(e) => setAppendFooter(e.target.checked)}
                      />
                      Append verification footer disclaimer to text completions
                    </label>
                  </div>
                  {appendFooter && (
                    <div className="field">
                      <label className="field__label">Footer Disclaimer Text</label>
                      <input
                        type="text"
                        className="input"
                        value={footerText}
                        onChange={(e) => setFooterText(e.target.value)}
                      />
                    </div>
                  )}
                </div>
              )}

              {/* 17. webhook-notifier UI */}
              {selectedPluginName === "webhook-notifier" && (
                <div className="stack stack--gap-md">
                  <div className="field">
                    <label className="field__label">Webhook Endpoint URL</label>
                    <input
                      type="url"
                      className="input"
                      value={alertWebhookUrl}
                      onChange={(e) => setAlertWebhookUrl(e.target.value)}
                      placeholder="https://hooks.slack.com/services/... or https://api.corp.internal/alert"
                      required
                    />
                  </div>
                  <div className="field">
                    <label className="field__label">Trigger Conditions</label>
                    <div style={{ display: "flex", flexDirection: "column", gap: "0.5rem", marginTop: "0.25rem" }}>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
                        <input
                          type="checkbox"
                          checked={notifyErrors}
                          onChange={(e) => setNotifyErrors(e.target.checked)}
                        />
                        Notify on HTTP Error Responses (&gt;= 400)
                      </label>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
                        <input
                          type="checkbox"
                          checked={notifySlow}
                          onChange={(e) => setNotifySlow(e.target.checked)}
                        />
                        Notify on Slow Requests
                      </label>
                    </div>
                  </div>
                  {notifySlow && (
                    <div className="field">
                      <label className="field__label">Slow Latency Threshold (Seconds)</label>
                      <input
                        type="number"
                        step="0.5"
                        min="0.5"
                        className="input"
                        value={slowThreshold}
                        onChange={(e) => setSlowThreshold(Number(e.target.value))}
                      />
                    </div>
                  )}
                  <div className="field">
                    <label className="field__label">Surge Token Threshold (0 = disabled)</label>
                    <input
                      type="number"
                      min="0"
                      className="input"
                      value={minTokensThreshold}
                      onChange={(e) => setMinTokensThreshold(Number(e.target.value))}
                    />
                  </div>
                </div>
              )}

              {/* 18. content-moderation UI */}
              {selectedPluginName === "content-moderation" && (
                <div className="stack stack--gap-md">
                  <div className="field">
                    <label className="field__label">Safety Categories to Block</label>
                    <div style={{ display: "flex", flexDirection: "column", gap: "0.5rem", marginTop: "0.25rem" }}>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
                        <input
                          type="checkbox"
                          checked={modBlockHate}
                          onChange={(e) => setModBlockHate(e.target.checked)}
                        />
                        Hate Speech &amp; Extremism
                      </label>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
                        <input
                          type="checkbox"
                          checked={modBlockViolence}
                          onChange={(e) => setModBlockViolence(e.target.checked)}
                        />
                        Weapons &amp; Violence Directives
                      </label>
                      <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
                        <input
                          type="checkbox"
                          checked={modBlockSelfHarm}
                          onChange={(e) => setModBlockSelfHarm(e.target.checked)}
                        />
                        Self-Harm &amp; Suicide Content
                      </label>
                    </div>
                  </div>
                  <div className="field">
                    <label className="field__label">Enforcement Action</label>
                    <select
                      className="select"
                      value={modAction}
                      onChange={(e) => setModAction(e.target.value)}
                    >
                      <option value="block">Block Request (400 Bad Request)</option>
                      <option value="warn">Warn Only (Emit X-Moderation-Warning header)</option>
                    </select>
                  </div>
                  <div className="field">
                    <label className="field__label">Custom Banned Keywords (Comma-separated)</label>
                    <input
                      type="text"
                      className="input"
                      value={customBannedWords}
                      onChange={(e) => setCustomBannedWords(e.target.value)}
                      placeholder="internal-codename, confidential-project"
                    />
                  </div>
                </div>
              )}

              {/* Fallback for unlisted generic plugins */}
              {![
                "rate-limiting",
                "budget-control",
                "pii-masking",
                "prompt-guard",
                "semantic-cache",
                "request-hedging",
                "header-transform",
                "retry-circuit-breaker",
                "langfuse-exporter",
                "audit-trail",
                "prometheus-metrics",
                "system-prompt-injector",
                "ip-restriction",
                "canary-traffic-splitter",
                "prompt-compression",
                "response-watermarking",
                "webhook-notifier",
                "content-moderation",
              ].includes(selectedPluginName) && (
                <div className="field">
                  <label className="field__label">Plugin Config (JSON)</label>
                  <textarea
                    className="input"
                    style={{ fontFamily: "monospace", minHeight: 120 }}
                    value={rawConfig}
                    onChange={(e) => setRawConfig(e.target.value)}
                  />
                </div>
              )}
            </>
          )}
        </div>

        {/* Priority & Active */}
        <div style={{ display: "flex", gap: "1.5rem" }}>
          <div className="field" style={{ flex: 1 }}>
            <label className="field__label">Execution Priority</label>
            <input
              type="number"
              className="input"
              value={priority}
              onChange={(e) => setPriority(Number(e.target.value))}
            />
            <span className="field__help">Lower number runs earlier in pipeline (e.g. 10 runs before 100).</span>
          </div>
          <div className="field" style={{ flex: 1, display: "flex", alignItems: "center", paddingTop: "1.5rem" }}>
            <label style={{ display: "flex", alignItems: "center", gap: "0.5rem", cursor: "pointer" }}>
              <input
                type="checkbox"
                checked={isActive}
                onChange={(e) => setIsActive(e.target.checked)}
              />
              <span style={{ fontWeight: 600 }}>Enable Binding</span>
            </label>
          </div>
        </div>

        <div style={{ display: "flex", justifyContent: "flex-end", gap: "0.75rem", marginTop: "1rem" }}>
          <button type="button" className="btn btn--outline" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button type="submit" className="btn btn--primary" disabled={submitting}>
            {submitting ? "Saving..." : binding ? "Update Binding" : "Create Binding"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
