// tree-integrity.js — TEST/DEV helper for editor command tests (M6)
// Verifies SDT invariants: unique IDs, reachability, no cycles, single parent,
// placement.parents, children.mode/min/max.
// This is a test helper only, not a runtime validation framework.

export function assertTreeIntegrity(document, { catalog, definitionForBlock } = {}) {
  if (!document || typeof document !== "object") throw new Error("assertTreeIntegrity: document required");
  const nodes = document.nodes || [];
  const seen = new Map(); // id -> count
  const parentMap = new Map(); // childId -> parentId (null for root)
  const visited = new Set();
  const stack = [];

  function walk(list, parentId) {
    for (const n of list || []) {
      if (!n || typeof n.id !== "string") throw new Error(`assertTreeIntegrity: missing id for node ${JSON.stringify(n)}`);
      // duplicate detection (exact once)
      const count = seen.get(n.id) || 0;
      seen.set(n.id, count + 1);
      if (count >= 1) throw new Error(`assertTreeIntegrity: duplicate ID ${n.id}`);
      // cycle detection via ancestor stack
      if (stack.includes(n.id)) throw new Error(`assertTreeIntegrity: cycle detected at ${n.id} via ${stack.join("->")}`);
      if (parentMap.has(n.id)) throw new Error(`assertTreeIntegrity: node ${n.id} has multiple parents (${parentMap.get(n.id)} and ${parentId})`);
      parentMap.set(n.id, parentId);
      stack.push(n.id);
      visited.add(n.id);
      // placement / children mode checks if catalog helpers provided
      if (definitionForBlock) {
        const def = definitionForBlock(n.block, n.version);
        if (def && def.schema && def.schema.children) {
          const rule = def.schema.children;
          const children = n.children || [];
          if (rule.mode === "none" && children.length !== 0) {
            throw new Error(`assertTreeIntegrity: ${n.block} mode none but has ${children.length} children`);
          }
          if (rule.min != null && children.length < rule.min) {
            throw new Error(`assertTreeIntegrity: ${n.block} (${n.id}) requires at least ${rule.min} children, has ${children.length}`);
          }
          if (rule.max != null && children.length > rule.max) {
            throw new Error(`assertTreeIntegrity: ${n.block} (${n.id}) allows at most ${rule.max} children, has ${children.length}`);
          }
          if (rule.mode === "allowed" && Array.isArray(rule.blocks)) {
            for (const ch of children) {
              if (!rule.blocks.includes(ch.block)) {
                throw new Error(`assertTreeIntegrity: ${ch.block} not allowed inside ${n.block} (${n.id})`);
              }
            }
          }
        }
        // placement.parents check for children (child placement must be satisfied by this parent)
        for (const ch of (n.children || [])) {
          const childDef = definitionForBlock(ch.block, ch.version);
          const parents = childDef?.schema?.placement?.parents;
          if (parents && parents.length > 0) {
            if (!parents.includes(n.block)) {
              throw new Error(`assertTreeIntegrity: ${ch.block} cannot be placed inside ${n.block} (allowed: ${parents.join(",")})`);
            }
          }
        }
      }
      // recurse
      walk(n.children || [], n.id);
      stack.pop();
    }
  }

  function throwRootPlacement(parents, n) {
    throw new Error(`assertTreeIntegrity: ${n.block} (${n.id}) cannot be placed at top level (requires inside ${parents.join(",")})`);
  }

  // root placement check: nodes at root must satisfy placement.parents (null parent)
  if (definitionForBlock) {
    for (const n of nodes) {
      const childDef = definitionForBlock(n.block, n.version);
      const parents = childDef?.schema?.placement?.parents;
      if (parents && parents.length > 0) {
        throwRootPlacement(parents, n);
      }
    }
  }

  walk(nodes, null);

  // every child reachable from root (already via walk); check that no duplicate ids hidden elsewhere
  // Also ensure visited size equals seen size (all reachable) — already.

  // Also validate every node ID occurs exactly once in editable SDT — already duplicate check and walk covers reachable.
  // No extra orphan checks needed because we only walk from root; if duplicate map size != visited size would catch, but extra check:
  const totalNodes = (() => {
    let c = 0;
    const w = (list) => { for (const x of list) { c++; w(x.children||[]); } };
    w(nodes);
    return c;
  })();
  if (totalNodes !== seen.size) throw new Error(`assertTreeIntegrity: total count mismatch ${totalNodes} vs unique ${seen.size}`);

  // parent children.mode already checked for existing parents; also check for nodes that are parents via catalog but not visited? handled.

  return true;
}
