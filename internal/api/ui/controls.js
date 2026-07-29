state.runtimeExecutions = [];
state.pendingControlAction = null;

loadAgents = async function loadControlledAgents() {
  state.agents = [];
  state.subagentExecutions = [];
  state.runtimeExecutions = [];
  const runtime = selectedRuntime();
  if (!runtime) {
    renderAgents();
    renderSubagentExecutions();
    return;
  }
  renderAgents(true);
  renderSubagentExecutions(true);
  try {
    const base = `/v1/runtime-instances/${encodeURIComponent(runtime.id)}`;
    const [agents, subagents, executions] = await Promise.all([
      api(`${base}/agents`),
      api(`${base}/subagent-executions`),
      api(`${base}/executions`),
    ]);
    state.agents = agents;
    state.subagentExecutions = subagents;
    state.runtimeExecutions = executions;
    showNotice("");
  } catch (error) {
    showNotice(`Could not load imported instance state: ${error.message}`);
  }
  renderAgents();
  renderSubagentExecutions();
  renderSummary();
};

const controlShowAccess = showAccess;
showAccess = async function showControlledAccess(agentID, agentName) {
  state.selectedAgentID = agentID;
  const agent = state.agents.find((item) => item.id === agentID);
  const runtime = selectedRuntime();
  $("#agentStatusButton").classList.add("hidden");
  $("#deleteAgentButton").classList.add("hidden");
  $("#openAccessEditor").classList.add("hidden");
  await controlShowAccess(agentID, agentName);
  if (!agent || runtime?.mode !== "control_enabled") return;

  if (runtime.runtime_type === "gantry") {
    $("#openAccessEditor").classList.remove("hidden");
    const statusButton = $("#agentStatusButton");
    statusButton.textContent = agent.status === "disabled" ? "Enable" : "Disable";
    statusButton.dataset.nextStatus = agent.status === "disabled" ? "enabled" : "disabled";
    statusButton.classList.remove("hidden");
  }
  if (runtime.runtime_type === "langgraph" && !agent.metadata?.runtime_deleted) {
    $("#deleteAgentButton").classList.remove("hidden");
  }
};

renderSubagentExecutions = function renderRuntimeActivity(loading = false) {
  const runtime = selectedRuntime();
  const langGraph = runtime?.runtime_type === "langgraph";
  const rows = langGraph ? state.runtimeExecutions : state.subagentExecutions;
  const owners = new Map(state.agents.map((agent) => [agent.runtime_agent_id, agent.name]));
  $("#runtimeActivityTitle").textContent = langGraph ? "Threads and runs" : "Subagent executions";
  $("#subagentExecutionCount").textContent = `${rows.length} execution${rows.length === 1 ? "" : "s"}`;
  if (loading) {
    $("#subagentExecutionTable").innerHTML = `<tr class="empty-row"><td colspan="7">Loading runtime activity...</td></tr>`;
    return;
  }
  if (!rows.length) {
    $("#subagentExecutionTable").innerHTML = `<tr class="empty-row"><td colspan="7">No runtime activity has been observed for this instance.</td></tr>`;
    return;
  }
  $("#subagentExecutionTable").innerHTML = rows.map((item) => {
    const kind = langGraph ? item.kind : (item.subagent_type || "subagent");
    const parent = langGraph ? item.parent_runtime_execution_id : item.parent_run_id;
    const context = langGraph
      ? (item.metadata?.langsmith_session_name || "LangGraph execution")
      : (item.description || item.summary || "No task description");
    const canCancel = langGraph && runtime.mode === "control_enabled" && item.kind === "run" &&
      ["pending", "running"].includes(item.status);
    return `<tr>
      <td><div class="agent-name"><strong>${escapeHTML(kind)}</strong><small>${escapeHTML(item.runtime_execution_id)}</small></div></td>
      <td>${escapeHTML(owners.get(item.runtime_agent_id) || item.runtime_agent_id || "Runtime")}</td>
      <td><span class="badge ${escapeHTML(item.status)}">${escapeHTML(item.status)}</span></td>
      <td><code>${escapeHTML(parent || "-")}</code></td>
      <td title="${escapeHTML(context)}">${escapeHTML(context)}</td>
      <td>${formatTime(item.observed_at)}</td>
      <td>${canCancel ? `<button class="text-button danger-text cancel-execution" data-execution-id="${escapeHTML(item.id)}">Cancel</button>` : ""}</td>
    </tr>`;
  }).join("");
  $$(".cancel-execution").forEach((button) => button.addEventListener("click", () =>
    openControlAction({ type: "cancel_execution", executionID: button.dataset.executionId })));
};

const controlRenderVerification = renderVerification;
renderVerification = function renderControlVerification() {
  controlRenderVerification();
  if (!state.testResult) return;
  const labels = {
    read_executions: "Read executions",
    set_agent_status: "Enable / disable",
    delete_agent: "Delete agent",
    cancel_execution: "Cancel execution",
  };
  $("#capabilities").insertAdjacentHTML("beforeend", Object.entries(labels)
    .map(([key, label]) => `<span class="capability ${state.testResult.capabilities?.[key] ? "on" : ""}">${label}</span>`)
    .join(""));
};

function openControlAction(action) {
  const agent = state.agents.find((item) => item.id === state.selectedAgentID);
  state.pendingControlAction = { ...action, agent };
  const labels = {
    set_status: action.status === "enabled" ? "Enable agent" : "Disable agent",
    delete_agent: "Delete assistant",
    cancel_execution: "Cancel run",
  };
  $("#controlActionTitle").textContent = labels[action.type];
  $("#controlActor").value = sessionStorage.getItem("capcom.syncActor") || "local-operator";
  $("#controlReason").value = action.type === "cancel_execution" ? "Stop active execution" : "Operator requested change";
  $("#controlDryRun").checked = action.type !== "cancel_execution";
  $("#controlConfirmationField").classList.toggle("hidden", action.type !== "delete_agent");
  $("#controlConfirmation").required = action.type === "delete_agent";
  $("#controlConfirmation").value = "";
  $("#controlConfirmation").placeholder = agent?.runtime_agent_id || "";
  $("#controlActionDialog").showModal();
}

async function submitControlAction(event) {
  event.preventDefault();
  const pending = state.pendingControlAction;
  if (!pending) return;
  const actor = $("#controlActor").value.trim();
  const reason = $("#controlReason").value.trim();
  const dryRun = $("#controlDryRun").checked;
  const payload = { actor, reason, dry_run: dryRun, idempotency_key: crypto.randomUUID() };
  let endpoint;
  if (pending.type === "set_status") {
    endpoint = `/v1/agents/${encodeURIComponent(pending.agent.id)}/actions/set-status`;
    payload.status = pending.status;
  } else if (pending.type === "delete_agent") {
    endpoint = `/v1/agents/${encodeURIComponent(pending.agent.id)}/actions/delete`;
    payload.confirmation = $("#controlConfirmation").value.trim();
  } else {
    endpoint = `/v1/runtime-executions/${encodeURIComponent(pending.executionID)}/actions/cancel`;
  }
  const button = $("#confirmControlActionButton");
  sessionStorage.setItem("capcom.syncActor", actor);
  button.disabled = true;
  button.textContent = dryRun ? "Validating..." : "Applying...";
  try {
    const action = await api(endpoint, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    $("#controlActionDialog").close();
    $("#accessDialog").close();
    showNotice(`Control action ${action.action_type} ${action.status}${dryRun ? " (validation only)" : ""}.`);
    await loadRuntimes();
  } catch (error) {
    showNotice(`Control action rejected: ${error.message}`);
  } finally {
    button.disabled = false;
    button.textContent = "Continue";
  }
}

$("#agentStatusButton").addEventListener("click", (event) =>
  openControlAction({ type: "set_status", status: event.currentTarget.dataset.nextStatus }));
$("#deleteAgentButton").addEventListener("click", () => openControlAction({ type: "delete_agent" }));
$("#controlActionForm").addEventListener("submit", submitControlAction);

loadAgents();
