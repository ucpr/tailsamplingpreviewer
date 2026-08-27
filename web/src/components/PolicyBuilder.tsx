import { useEffect, useState } from "react";
import { ApiError, putPolicy } from "../api";
import { useAppState } from "../state";
import { fieldsForType, POLICY_TYPES, subFieldsForType, SUB_POLICY_TYPES } from "../policyDefaults";
import { TypeFields } from "./TypeFields";
import type { AndSubPolicyCfg, PolicyCfg, PolicyType, PolicyView } from "../types";

interface AndSubPolicyListProps {
  subPolicies: AndSubPolicyCfg[];
  onChange: (subPolicies: AndSubPolicyCfg[]) => void;
}

function AndSubPolicyList({ subPolicies, onChange }: AndSubPolicyListProps) {
  const updateSub = (index: number, patch: Partial<AndSubPolicyCfg>) =>
    onChange(subPolicies.map((s, i) => (i === index ? { ...s, ...patch } : s)));
  const removeSub = (index: number) => onChange(subPolicies.filter((_, i) => i !== index));
  const addSub = () =>
    onChange([
      ...subPolicies,
      { name: `condition-${subPolicies.length + 1}`, type: "always_sample", ...subFieldsForType("always_sample") },
    ]);
  const changeSubType = (index: number, type: AndSubPolicyCfg["type"]) =>
    updateSub(index, { type, ...subFieldsForType(type) });

  return (
    <div className="and-sub-policy-list">
      <p className="field-hint">ALL conditions below must match (AND).</p>
      {subPolicies.map((sub, index) => (
        <div className="policy-editor policy-editor--nested" key={index}>
          <div className="policy-editor__row">
            <input
              type="text"
              className="policy-editor__name"
              value={sub.name}
              placeholder="condition name"
              onChange={(e) => updateSub(index, { name: e.target.value })}
            />
            <select
              value={sub.type}
              onChange={(e) => changeSubType(index, e.target.value as AndSubPolicyCfg["type"])}
            >
              {SUB_POLICY_TYPES.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
            <button type="button" className="remove-button" onClick={() => removeSub(index)}>
              Remove
            </button>
          </div>
          <TypeFields type={sub.type} cfg={sub} onChange={(patch) => updateSub(index, patch)} />
        </div>
      ))}
      <button type="button" className="add-policy-button add-policy-button--nested" onClick={addSub}>
        + Add condition
      </button>
    </div>
  );
}

interface PolicyEditorProps {
  policy: PolicyCfg;
  onChange: (patch: Partial<PolicyCfg>) => void;
  onRemove: () => void;
}

function PolicyEditor({ policy, onChange, onRemove }: PolicyEditorProps) {
  const changeType = (type: PolicyType) => onChange({ type, ...fieldsForType(type) });

  return (
    <div className="policy-editor">
      <div className="policy-editor__row">
        <input
          type="text"
          className="policy-editor__name"
          value={policy.name}
          placeholder="policy name"
          onChange={(e) => onChange({ name: e.target.value })}
        />
        <select value={policy.type} onChange={(e) => changeType(e.target.value as PolicyType)}>
          {POLICY_TYPES.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
        <button type="button" className="remove-button" onClick={onRemove}>
          Remove
        </button>
      </div>

      <TypeFields type={policy.type} cfg={policy} onChange={onChange} />

      {policy.type === "and" && (
        <AndSubPolicyList
          subPolicies={policy.and?.and_sub_policy ?? []}
          onChange={(and_sub_policy) => onChange({ and: { and_sub_policy } })}
        />
      )}
    </div>
  );
}

export function PolicyBuilder() {
  const { state, setPolicy, clearPolicySuggestions } = useAppState();
  const [draft, setDraft] = useState<PolicyView | null>(null);
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  useEffect(() => {
    if (!dirty && state.policy) setDraft(state.policy);
  }, [state.policy, dirty]);

  // Merge in policies proposed by the AI policy assistant (Trace Detail),
  // then drain the queue so they're only applied once.
  useEffect(() => {
    const suggestions = state.pendingPolicySuggestions;
    if (!suggestions || suggestions.length === 0) return;
    setDraft((d) => (d ? { ...d, policies: [...d.policies, ...suggestions] } : d));
    setDirty(true);
    clearPolicySuggestions();
  }, [state.pendingPolicySuggestions, clearPolicySuggestions]);

  if (!draft) {
    return (
      <section className="panel">
        <h2 className="panel__title">Policy Builder</h2>
        <p className="app-header__empty">Loading policy…</p>
      </section>
    );
  }

  const mutate = (fn: (d: PolicyView) => PolicyView) => {
    setDraft((d) => (d ? fn(d) : d));
    setDirty(true);
  };

  const updatePolicy = (index: number, patch: Partial<PolicyCfg>) =>
    mutate((d) => ({
      ...d,
      policies: d.policies.map((p, i) => (i === index ? { ...p, ...patch } : p)),
    }));

  const removePolicy = (index: number) =>
    mutate((d) => ({ ...d, policies: d.policies.filter((_, i) => i !== index) }));

  const addPolicy = () =>
    mutate((d) => ({
      ...d,
      policies: [
        ...d.policies,
        {
          name: `policy-${d.policies.length + 1}`,
          type: "always_sample" as PolicyType,
          ...fieldsForType("always_sample"),
        },
      ],
    }));

  const handleSave = async () => {
    setSaving(true);
    setSaveError(null);
    try {
      const saved = await putPolicy(draft);
      setPolicy(saved);
      setDraft(saved);
      setDirty(false);
    } catch (err) {
      setSaveError(err instanceof ApiError ? err.message : "Save failed");
    } finally {
      setSaving(false);
    }
  };

  const handleDiscard = () => {
    if (state.policy) setDraft(state.policy);
    setDirty(false);
    setSaveError(null);
  };

  return (
    <section className="panel panel--policy-builder">
      <div className="panel__header-row">
        <h2 className="panel__title">Policy Builder</h2>
        {dirty && <span className="dirty-flag">unsaved changes</span>}
      </div>

      <label className="decision-wait-field">
        decision_wait
        <input
          type="text"
          value={draft.decision_wait}
          onChange={(e) => mutate((d) => ({ ...d, decision_wait: e.target.value }))}
        />
      </label>

      <p className="field-hint">A trace is KEPT if ANY policy below matches.</p>

      <div className="policy-list">
        {draft.policies.map((policy, index) => (
          <PolicyEditor
            key={index}
            policy={policy}
            onChange={(patch) => updatePolicy(index, patch)}
            onRemove={() => removePolicy(index)}
          />
        ))}
      </div>

      <button type="button" className="add-policy-button" onClick={addPolicy}>
        + Add policy
      </button>

      {saveError && <p className="notice notice--error">{saveError}</p>}

      <div className="button-row">
        <button type="button" onClick={handleSave} disabled={saving || !dirty}>
          {saving ? "Saving…" : "Save policy"}
        </button>
        <button type="button" onClick={handleDiscard} disabled={saving || !dirty}>
          Discard changes
        </button>
      </div>
    </section>
  );
}
