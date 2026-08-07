import assert from "node:assert/strict"
import { describe, it } from "node:test"

import {
  buildAgentTopology,
  collapseAgentTopology,
  filterAgentTopology,
} from "./agent-topology.ts"
import type {
  AgentDelegation,
  AgentKind,
  PersistedAgent,
  RuntimeInstance,
} from "./api-types.ts"

const instance = runtimeInstance("runtime-1", "Gantry Local")

describe("buildAgentTopology", () => {
  it("places multiple main agents before their delegated and standalone agents", () => {
    const agents = [
      agent("z-main", "Zulu Main", "main"),
      agent("z-child", "Zulu Delegate"),
      agent("a-main", "Alpha Main", "main"),
      agent("a-child", "Alpha Delegate"),
      agent("standalone", "Utility"),
    ]
    const rows = buildAgentTopology(
      agents,
      [
        delegation("a-main", "a-child"),
        delegation("z-main", "z-child"),
      ],
      [instance]
    )

    assert.deepEqual(rows.map((row) => row.agent.runtime_agent_id), [
      "a-main",
      "a-child",
      "z-main",
      "z-child",
      "standalone",
    ])
    assert.deepEqual(rows.map((row) => row.role), [
      "main",
      "delegated",
      "main",
      "delegated",
      "standalone",
    ])
  })

  it("terminates closed cycles and labels every participant", () => {
    const rows = buildAgentTopology(
      [agent("a", "Agent A"), agent("b", "Agent B")],
      [delegation("a", "b"), delegation("b", "a")],
      [instance]
    )

    assert.equal(rows.length, 2)
    assert.equal(rows.every((row) => row.inCycle), true)
    assert.deepEqual(rows.map((row) => row.agent.runtime_agent_id), ["a", "b"])
  })

  it("keeps unresolved delegates on their orchestrator without inventing an agent", () => {
    const unresolved = delegation("main", undefined, "missing-settings")
    const rows = buildAgentTopology(
      [agent("main", "Main", "main"), agent("utility", "Utility")],
      [unresolved],
      [instance]
    )

    assert.equal(rows.length, 2)
    assert.deepEqual(rows[0].unresolvedDelegations, [unresolved])
    assert.equal(rows[1].role, "standalone")
  })

  it("renders a multi-orchestrator delegate once and retains all incoming edges", () => {
    const rows = buildAgentTopology(
      [
        agent("alpha", "Alpha Main", "main"),
        agent("beta", "Beta Main", "main"),
        agent("shared", "Shared Researcher"),
      ],
      [delegation("alpha", "shared"), delegation("beta", "shared")],
      [instance]
    )
    const shared = rows.find((row) => row.agent.runtime_agent_id === "shared")

    assert.equal(
      rows.filter((row) => row.agent.runtime_agent_id === "shared").length,
      1
    )
    assert.deepEqual(shared?.delegatedBy.map((agent) => agent.name), [
      "Alpha Main",
      "Beta Main",
    ])
  })
})

describe("collapseAgentTopology", () => {
  it("hides only descendants of a collapsed branch", () => {
    const rows = buildAgentTopology(
      [
        agent("alpha", "Alpha Main", "main"),
        agent("alpha-child", "Alpha Child"),
        agent("beta", "Beta Main", "main"),
        agent("beta-child", "Beta Child"),
      ],
      [delegation("alpha", "alpha-child"), delegation("beta", "beta-child")],
      [instance]
    )

    const visible = collapseAgentTopology(rows, new Set(["id-alpha"]))

    assert.deepEqual(visible.map((row) => row.agent.runtime_agent_id), [
      "alpha",
      "beta",
      "beta-child",
    ])
  })
})

describe("filterAgentTopology", () => {
  it("retains every orchestrator as context when a delegate matches", () => {
    const rows = buildAgentTopology(
      [
        agent("alpha", "Alpha Main", "main"),
        agent("beta", "Beta Main", "main"),
        agent("shared", "Shared Researcher"),
      ],
      [delegation("alpha", "shared"), delegation("beta", "shared")],
      [instance]
    )
    const filtered = filterAgentTopology(rows, [instance], "researcher")

    assert.deepEqual(filtered.map((row) => row.agent.runtime_agent_id), [
      "alpha",
      "shared",
      "beta",
    ])
    assert.deepEqual(filtered.map((row) => row.contextOnly), [true, false, true])
  })
})

function agent(
  runtimeAgentID: string,
  name: string,
  kind: AgentKind = "registered"
): PersistedAgent {
  return {
    id: `id-${runtimeAgentID}`,
    name,
    status: "enabled",
    kind,
    runtime_connection_id: instance.id,
    runtime_agent_id: runtimeAgentID,
    freshness: "live",
    observed_at: "2026-08-03T00:00:00Z",
  }
}

function delegation(
  orchestratorRuntimeAgentID: string,
  delegateRuntimeAgentID?: string,
  delegateRef = delegateRuntimeAgentID ?? "unresolved"
): AgentDelegation {
  return {
    id: `edge-${orchestratorRuntimeAgentID}-${delegateRef}`,
    runtime_connection_id: instance.id,
    orchestrator_runtime_agent_id: orchestratorRuntimeAgentID,
    delegate_runtime_agent_id: delegateRuntimeAgentID,
    delegate_ref: delegateRef,
    configured: true,
    resolved: Boolean(delegateRuntimeAgentID),
    revision: 1,
    status: "active",
    observed_at: "2026-08-03T00:00:00Z",
  }
}

function runtimeInstance(id: string, displayName: string): RuntimeInstance {
  return {
    id,
    name: id,
    display_name: displayName,
    environment: "development",
    labels: {},
    runtime_type: "gantry",
    mode: "read_only",
    status: "active",
    endpoint: "http://runtime.internal",
    auth_ref: "secret-ref",
    created_at: "2026-08-03T00:00:00Z",
    updated_at: "2026-08-03T00:00:00Z",
    sync_enabled: true,
    sync_interval_seconds: 60,
    endpoints: [],
  }
}
