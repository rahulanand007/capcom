package api

import "net/http"

func handleListRuntimeDiagnostics(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeSync == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "sync_not_configured"})
			return
		}
		items, err := cfg.RuntimeSync.ListRuntimeDiagnostics(r.Context(), r.PathValue("id"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			out = append(out, map[string]any{"id": item.ID, "runtime_connection_id": item.RuntimeConnectionID,
				"check_id": item.CheckID, "status": item.Status, "message": item.Message,
				"observed_at": item.ObservedAt, "metadata": item.Metadata})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func handleListRuntimeInventory(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeSync == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "sync_not_configured"})
			return
		}
		items, err := cfg.RuntimeSync.ListRuntimeInventory(r.Context(), r.PathValue("id"), r.URL.Query().Get("kind"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			out = append(out, map[string]any{"id": item.ID, "runtime_connection_id": item.RuntimeConnectionID,
				"runtime_item_id": item.RuntimeItemID, "kind": item.Kind, "name": item.Name, "status": item.Status,
				"provider": item.Provider, "source": item.Source, "observed_at": item.ObservedAt, "metadata": item.Metadata})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func handleListRuntimeCapabilities(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeSync == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "sync_not_configured"})
			return
		}
		items, err := cfg.RuntimeSync.ListRuntimeCapabilities(r.Context(), r.PathValue("id"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			out = append(out, map[string]any{"id": item.ID, "runtime_connection_id": item.RuntimeConnectionID,
				"runtime_capability_id": item.RuntimeCapabilityID, "version": item.Version, "name": item.Name,
				"category": item.Category, "risk": item.Risk, "can": item.Can, "cannot": item.Cannot,
				"source": item.Source, "observed_at": item.ObservedAt, "metadata": item.Metadata})
		}
		writeJSON(w, http.StatusOK, out)
	}
}
