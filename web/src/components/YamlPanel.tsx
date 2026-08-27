import { useEffect, useState } from "react";
import { ApiError, getPolicyYAML, putPolicyYAML } from "../api";
import { useAppState } from "../state";

function download(filename: string, contents: string): void {
  const blob = new Blob([contents], { type: "text/yaml" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

export function YamlPanel() {
  const { state, setPolicy } = useAppState();
  const [exportedYaml, setExportedYaml] = useState("");
  const [copyLabel, setCopyLabel] = useState("Copy");
  const [importText, setImportText] = useState("");
  const [importError, setImportError] = useState<string | null>(null);
  const [importing, setImporting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    getPolicyYAML()
      .then((text) => {
        if (!cancelled) setExportedYaml(text);
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [state.policy]);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(exportedYaml);
      setCopyLabel("Copied!");
      setTimeout(() => setCopyLabel("Copy"), 1500);
    } catch {
      setCopyLabel("Copy failed");
      setTimeout(() => setCopyLabel("Copy"), 1500);
    }
  };

  const handleImport = async () => {
    setImporting(true);
    setImportError(null);
    try {
      const policy = await putPolicyYAML(importText);
      setPolicy(policy);
    } catch (err) {
      setImportError(err instanceof ApiError ? err.message : "Import failed");
    } finally {
      setImporting(false);
    }
  };

  return (
    <section className="panel panel--yaml">
      <h2 className="panel__title">YAML</h2>

      <div className="yaml-export">
        <div className="panel__header-row">
          <h3 className="panel__subtitle">Export</h3>
          <div className="button-row">
            <button type="button" onClick={handleCopy}>
              {copyLabel}
            </button>
            <button type="button" onClick={() => download("tail_sampling.yaml", exportedYaml)}>
              Download
            </button>
          </div>
        </div>
        <pre className="yaml-view">{exportedYaml}</pre>
      </div>

      <div className="yaml-import">
        <h3 className="panel__subtitle">Import</h3>
        <textarea
          className="yaml-textarea"
          value={importText}
          onChange={(e) => setImportText(e.target.value)}
          placeholder={"processors:\n  tail_sampling:\n    decision_wait: 30s\n    policies:\n      - name: errors\n        type: status_code\n        status_code:\n          status_codes: [ERROR]"}
          spellCheck={false}
        />
        {importError && <p className="notice notice--error">{importError}</p>}
        <div className="button-row">
          <button type="button" onClick={() => setImportText(exportedYaml)}>
            Load current into editor
          </button>
          <button type="button" onClick={handleImport} disabled={importing || importText.trim() === ""}>
            {importing ? "Importing…" : "Import"}
          </button>
        </div>
      </div>
    </section>
  );
}
