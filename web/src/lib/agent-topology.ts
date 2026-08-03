import type {
  AgentDelegation,
  PersistedAgent,
  RuntimeInstance,
} from "@/lib/api-types"

export type AgentTopologyRole = "main" | "delegated" | "standalone"

export type AgentTopologyRow = {
  agent: PersistedAgent
  depth: number
  role: AgentTopologyRole
  delegatedBy: PersistedAgent[]
  unresolvedDelegations: AgentDelegation[]
  inCycle: boolean
  contextOnly: boolean
}

type InstanceGraph = {
  agents: PersistedAgent[]
  delegations: AgentDelegation[]
}

export function buildAgentTopology(
  agents: PersistedAgent[],
  delegations: AgentDelegation[],
  instances: RuntimeInstance[]
): AgentTopologyRow[] {
  const graphs = new Map<string, InstanceGraph>()

  for (const agent of agents) {
    const graph = graphs.get(agent.runtime_connection_id) ?? {
      agents: [],
      delegations: [],
    }
    graph.agents.push(agent)
    graphs.set(agent.runtime_connection_id, graph)
  }
  for (const delegation of delegations) {
    const graph = graphs.get(delegation.runtime_connection_id)
    if (graph) {
      graph.delegations.push(delegation)
    }
  }

  const instanceById = new Map(instances.map((instance) => [instance.id, instance]))
  return [...graphs.entries()]
    .sort(([left], [right]) =>
      instanceLabel(left, instanceById).localeCompare(
        instanceLabel(right, instanceById),
        undefined,
        { sensitivity: "base" }
      )
    )
    .flatMap(([, graph]) => buildInstanceTopology(graph))
}

export function filterAgentTopology(
  rows: AgentTopologyRow[],
  instances: RuntimeInstance[],
  query: string
): AgentTopologyRow[] {
  const needle = query.trim().toLocaleLowerCase()
  if (!needle) {
    return rows
  }

  const instanceById = new Map(instances.map((instance) => [instance.id, instance]))
  const rowByAgentID = new Map(rows.map((row) => [row.agent.id, row]))
  const matched = new Set<string>()
  const included = new Set<string>()

  for (const row of rows) {
    const instance = instanceById.get(row.agent.runtime_connection_id)
    const haystack = [
      row.agent.name,
      row.agent.runtime_agent_id,
      row.agent.kind,
      row.agent.status,
      row.agent.freshness,
      row.role,
      instance?.runtime_type,
      instance?.display_name,
      instance?.name,
      ...row.delegatedBy.flatMap((agent) => [agent.name, agent.runtime_agent_id]),
      ...row.unresolvedDelegations.flatMap((delegation) => [
        delegation.display_name,
        delegation.delegate_ref,
      ]),
    ]
      .filter(Boolean)
      .join(" ")
      .toLocaleLowerCase()

    if (haystack.includes(needle)) {
      matched.add(row.agent.id)
      included.add(row.agent.id)
    }
  }

  const pending = [...matched]
  while (pending.length) {
    const agentID = pending.pop()
    if (!agentID) continue
    const row = rowByAgentID.get(agentID)
    if (!row) continue
    for (const orchestrator of row.delegatedBy) {
      if (!included.has(orchestrator.id)) {
        included.add(orchestrator.id)
        pending.push(orchestrator.id)
      }
    }
  }

  return rows
    .filter((row) => included.has(row.agent.id))
    .map((row) => ({ ...row, contextOnly: !matched.has(row.agent.id) }))
}

function buildInstanceTopology(graph: InstanceGraph): AgentTopologyRow[] {
  const agents = [...graph.agents].sort(compareAgents)
  const byRuntimeID = new Map(agents.map((agent) => [agent.runtime_agent_id, agent]))
  const outgoing = new Map<string, Set<string>>()
  const incoming = new Map<string, Set<string>>()
  const unresolved = new Map<string, AgentDelegation[]>()

  for (const delegation of graph.delegations) {
    const orchestrator = byRuntimeID.get(delegation.orchestrator_runtime_agent_id)
    if (!orchestrator) continue

    const delegate = delegation.delegate_runtime_agent_id
      ? byRuntimeID.get(delegation.delegate_runtime_agent_id)
      : undefined
    if (!delegate) {
      const items = unresolved.get(orchestrator.id) ?? []
      items.push(delegation)
      unresolved.set(orchestrator.id, items)
      continue
    }

    addEdge(outgoing, orchestrator.id, delegate.id)
    addEdge(incoming, delegate.id, orchestrator.id)
  }

  const cyclic = findCyclicAgents(agents, outgoing)
  const emitted = new Set<string>()
  const result: AgentTopologyRow[] = []
  const byID = new Map(agents.map((agent) => [agent.id, agent]))

  const emit = (agent: PersistedAgent, depth: number) => {
    if (emitted.has(agent.id)) return false
    emitted.add(agent.id)

    const orchestrators = [...(incoming.get(agent.id) ?? [])]
      .map((id) => byID.get(id))
      .filter((item): item is PersistedAgent => Boolean(item))
      .sort(compareAgents)
    result.push({
      agent,
      depth,
      role: topologyRole(agent, orchestrators.length),
      delegatedBy: orchestrators,
      unresolvedDelegations: [...(unresolved.get(agent.id) ?? [])].sort(
        compareDelegations
      ),
      inCycle: cyclic.has(agent.id),
      contextOnly: false,
    })

    return true
  }

  const emitDescendants = (roots: PersistedAgent[]) => {
    const queue = roots.map((agent) => ({ agent, depth: 0 }))
    for (let cursor = 0; cursor < queue.length; cursor += 1) {
      const current = queue[cursor]
      const delegates = [...(outgoing.get(current.agent.id) ?? [])]
        .map((id) => byID.get(id))
        .filter((item): item is PersistedAgent => Boolean(item))
        .filter((item) => item.kind !== "main")
        .sort(compareAgents)
      for (const delegate of delegates) {
        if (emit(delegate, current.depth + 1)) {
          queue.push({ agent: delegate, depth: current.depth + 1 })
        }
      }
    }
  }

  const mainRoots = agents.filter((agent) => agent.kind === "main")
  const standaloneRoots = agents.filter(
    (agent) => agent.kind !== "main" && !(incoming.get(agent.id)?.size)
  )
  mainRoots.forEach((root) => emit(root, 0))
  emitDescendants(mainRoots)

  for (const root of standaloneRoots) {
    if (emit(root, 0)) emitDescendants([root])
  }

  // A closed cycle has no root. Emit it deterministically without recursing forever.
  for (const agent of agents) {
    if (emit(agent, 0)) emitDescendants([agent])
  }

  return result
}

function topologyRole(
  agent: PersistedAgent,
  orchestratorCount: number
): AgentTopologyRole {
  if (agent.kind === "main") return "main"
  return orchestratorCount > 0 ? "delegated" : "standalone"
}

function findCyclicAgents(
  agents: PersistedAgent[],
  outgoing: Map<string, Set<string>>
) {
  let index = 0
  const indices = new Map<string, number>()
  const lowLinks = new Map<string, number>()
  const stack: string[] = []
  const onStack = new Set<string>()
  const cyclic = new Set<string>()

  const connect = (agentID: string) => {
    indices.set(agentID, index)
    lowLinks.set(agentID, index)
    index += 1
    stack.push(agentID)
    onStack.add(agentID)

    for (const delegateID of outgoing.get(agentID) ?? []) {
      if (!indices.has(delegateID)) {
        connect(delegateID)
        lowLinks.set(
          agentID,
          Math.min(lowLinks.get(agentID)!, lowLinks.get(delegateID)!)
        )
      } else if (onStack.has(delegateID)) {
        lowLinks.set(
          agentID,
          Math.min(lowLinks.get(agentID)!, indices.get(delegateID)!)
        )
      }
    }

    if (lowLinks.get(agentID) !== indices.get(agentID)) return
    const component: string[] = []
    let member: string | undefined
    do {
      member = stack.pop()
      if (!member) break
      onStack.delete(member)
      component.push(member)
    } while (member !== agentID)

    if (
      component.length > 1 ||
      (component.length === 1 && outgoing.get(component[0])?.has(component[0]))
    ) {
      component.forEach((id) => cyclic.add(id))
    }
  }

  for (const agent of agents) {
    if (!indices.has(agent.id)) connect(agent.id)
  }
  return cyclic
}

function addEdge(map: Map<string, Set<string>>, from: string, to: string) {
  const values = map.get(from) ?? new Set<string>()
  values.add(to)
  map.set(from, values)
}

function compareAgents(left: PersistedAgent, right: PersistedAgent) {
  return (
    left.name.localeCompare(right.name, undefined, { sensitivity: "base" }) ||
    left.runtime_agent_id.localeCompare(right.runtime_agent_id)
  )
}

function compareDelegations(left: AgentDelegation, right: AgentDelegation) {
  return (left.display_name || left.delegate_ref).localeCompare(
    right.display_name || right.delegate_ref,
    undefined,
    { sensitivity: "base" }
  )
}

function instanceLabel(
  id: string,
  instances: Map<string, RuntimeInstance>
) {
  const instance = instances.get(id)
  return `${instance?.runtime_type ?? ""} ${instance?.display_name ?? instance?.name ?? id}`
}
