// move.test.js — M6 moveNode behavioral tests
import { describe, it, beforeEach } from "node:test";
import assert from "node:assert/strict";

const catalog = [
  { block: "core/section", version: 1, displayName: "Section", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "any", min: 0 }, editor: { category: "layout" } } },
  { block: "core/stack", version: 1, displayName: "Stack", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "any", min: 0 }, editor: { category: "layout" } } },
  { block: "core/grid", version: 1, displayName: "Grid", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "any", min: 0 }, editor: { category: "layout" } } },
  { block: "core/heading", version: 1, displayName: "Heading", schema: { props: { type: "object", properties: { text: { type: "string", default: "Hello" } } }, settings: { type: "object", properties: {} }, children: { mode: "none" }, editor: { category: "text" } } },
  { block: "core/text", version: 1, displayName: "Text", schema: { props: { type: "object", properties: { text: { type: "string", default: "" } } }, settings: { type: "object", properties: {} }, children: { mode: "none" }, editor: { category: "text" } } },
  { block: "core/button", version: 1, displayName: "Button", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "none" }, editor: { category: "design" } } },
  { block: "core/accordion", version: 1, displayName: "Accordion", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "allowed", blocks: ["core/accordion-item"], min: 1, max: 3 }, editor: { category: "design" } } },
  { block: "core/accordion-item", version: 1, displayName: "Accordion Item", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "any", min: 0 }, placement: { parents: ["core/accordion"] }, editor: { category: "design" } } },
  { block: "core/button-group", version: 1, displayName: "Button Group", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "allowed", blocks: ["core/button"], min: 1, max: 1 }, editor: { category: "design" } } },
  { block: "core/image", version: 1, displayName: "Image", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "none" }, editor: { category: "media" } } },
  { block: "core/hidden-block", version: 1, displayName: "Hidden", hidden: true, schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "none" }, editor: { category: "design" } } },
];

globalThis.document = {
  getElementById: (id) => {
    if (id === "editor-v2-bootstrap") return { textContent: JSON.stringify({ document: { version: 1, nodes: [] }, catalog, definitions: catalog, resource: { id: "e1", type: "entry" }, previewUrl: "/admin/editor/preview" }) };
    return null;
  },
  createElement: () => ({ style: {}, setAttribute: () => {}, append: () => {} }),
  querySelector: () => null,
  querySelectorAll: () => [],
};
globalThis.window = globalThis;
globalThis.window.location = { origin: "http://localhost", search: "" };
globalThis.location = globalThis.window.location;
try { globalThis.crypto.getRandomValues = (arr)=>{ for(let i=0;i<arr.length;i++) arr[i]=Math.floor(Math.random()*256); return arr;}} catch(_){}

const stateMod = await import("./state.js");
const insMod = await import("./insertion.js");
const cmdMod = await import("./commands.js");
const treeMod = await import("./tree-integrity.js");
const { state, findDocumentNode, definitionForBlock } = stateMod;
const { canMove, findSource, canInsert } = insMod;
const { createNode, moveNode } = cmdMod;
const { assertTreeIntegrity } = treeMod;

function reset(nodes){
  state.document = { version:1, nodes: JSON.parse(JSON.stringify(nodes)) };
  const walk = (list)=>{ for(const n of list){ n.children ||= []; walk(n.children); } };
  walk(state.document.nodes);
  state.selection=null;
  delete state.__pendingSelectionIds;
}

function mk(block,id){
  const def = definitionForBlock(block);
  const n = createNode(def);
  n.id=id;
  return n;
}
function countNodes(nodes){
  let c=0;
  for(const n of nodes){ c++; c+= countNodes(n.children||[]); }
  return c;
}
function collectIds(nodes, out=[]){
  for(const n of nodes){ out.push(n.id); collectIds(n.children||[],out); }
  return out;
}

describe("moveNode - spec §59-71", ()=>{
  beforeEach(()=> reset([]));

  it("59 root reorder forward A,B,C move B after C => A,C,B", ()=>{
    const A=mk("core/heading","A"), B=mk("core/heading","B"), C=mk("core/heading","C");
    reset([A,B,C]);
    const res=moveNode({nodeId:"B", parentId:null, index:3});
    assert.equal(res.ok,true);
    assert.equal(res.unchanged,undefined);
    assert.deepEqual(state.document.nodes.map(n=>n.id), ["A","C","B"]);
    // stable IDs
    assert.equal(collectIds(state.document.nodes).includes("B"),true);
  });

  it("60 same parent backward C before A => C,A,B", ()=>{
    const A=mk("core/heading","A"), B=mk("core/heading","B"), C=mk("core/heading","C");
    reset([A,B,C]);
    const res=moveNode({nodeId:"C", parentId:null, index:0});
    assert.equal(res.ok,true);
    assert.deepEqual(state.document.nodes.map(n=>n.id), ["C","A","B"]);
  });

  it("61 no-op before/after self", ()=>{
    const A=mk("core/heading","A"), B=mk("core/heading","B"), C=mk("core/heading","C");
    reset([A,B,C]);
    const before=JSON.stringify(state.document);
    const r1=moveNode({nodeId:"B", parentId:null, index:1});
    assert.equal(r1.ok,true);
    assert.equal(r1.unchanged,true);
    assert.equal(JSON.stringify(state.document), before);
    const r2=moveNode({nodeId:"B", parentId:null, index:2});
    assert.equal(r2.ok,true);
    assert.equal(r2.unchanged,true);
    assert.equal(JSON.stringify(state.document), before);
  });

  it("62 cross-parent A[X,Y] B[Z] move Y to B end", ()=>{
    const secA={id:"A", block:"core/section", version:1, props:{}, settings:{}, children:[{id:"X", block:"core/heading", version:1, props:{}, settings:{}, children:[]},{id:"Y", block:"core/text", version:1, props:{}, settings:{}, children:[]}]};
    const secB={id:"B", block:"core/section", version:1, props:{}, settings:{}, children:[{id:"Z", block:"core/heading", version:1, props:{}, settings:{}, children:[]}]};
    reset([secA,secB]);
    const res=moveNode({nodeId:"Y", parentId:"B", index:1});
    assert.equal(res.ok,true);
    assert.equal(state.document.nodes[0].children.length,1);
    assert.equal(state.document.nodes[0].children[0].id,"X");
    assert.equal(state.document.nodes[1].children.length,2);
    assert.equal(state.document.nodes[1].children[1].id,"Y");
  });

  it("63 root to container", ()=>{
    const sec={id:"sec1", block:"core/section", version:1, props:{}, settings:{}, children:[{id:"txt1", block:"core/text", version:1, props:{}, settings:{}, children:[]}]};
    const h=mk("core/heading","h1");
    reset([h,sec]);
    const res=moveNode({nodeId:"h1", parentId:"sec1", index:1});
    assert.equal(res.ok,true);
    assert.equal(state.document.nodes.length,1);
    assert.equal(state.document.nodes[0].children.length,2);
  });

  it("64 container to root", ()=>{
    const sec={id:"sec1", block:"core/section", version:1, props:{}, settings:{}, children:[mk("core/heading","h1")]};
    reset([sec]);
    const res=moveNode({nodeId:"h1", parentId:null, index:1});
    assert.equal(res.ok,true);
    assert.equal(state.document.nodes.length,2);
    assert.equal(state.document.nodes[1].id,"h1");
  });

  it("65 cycle protection", ()=>{
    const C={id:"C", block:"core/section", version:1, props:{}, settings:{}, children:[]};
    const B={id:"B", block:"core/section", version:1, props:{}, settings:{}, children:[C]};
    const A={id:"A", block:"core/section", version:1, props:{}, settings:{}, children:[B]};
    reset([A]);
    assert.equal(moveNode({nodeId:"A", parentId:"B", index:0}).ok,false);
    assert.equal(moveNode({nodeId:"A", parentId:"C", index:0}).ok,false);
    assert.equal(moveNode({nodeId:"B", parentId:"C", index:0}).ok,false);
  });

  it("66 source min", ()=>{
    const item={id:"X", block:"core/accordion-item", version:1, props:{}, settings:{}, children:[]};
    const acc={id:"acc", block:"core/accordion", version:1, props:{}, settings:{}, children:[item]};
    reset([acc]);
    const r1=moveNode({nodeId:"X", parentId:null, index:1});
    assert.equal(r1.ok,false);
    assert.match(r1.reason,/requires at least/);
    assertTreeIntegrity(state.document, { definitionForBlock });
    // add Y then reorder inside same accordion is legal, but moving to root/section still illegal due placement
    acc.children.push({id:"Y", block:"core/accordion-item", version:1, props:{}, settings:{}, children:[]});
    reset([acc]);
    const rRoot=moveNode({nodeId:"X", parentId:null, index:1});
    assert.equal(rRoot.ok,false);
    assert.match(rRoot.reason,/cannot be placed/);
    const rReorder=moveNode({nodeId:"X", parentId:"acc", index:1});
    assert.equal(rReorder.ok,true);
    assertTreeIntegrity(state.document, { definitionForBlock });
    // also placement: cannot move to Section/Stack
    reset([acc, {id:"sec1", block:"core/section", version:1, props:{}, settings:{}, children:[]}]);
    // need fresh acc with 2 items for min to allow leaving
    const acc2={id:"acc", block:"core/accordion", version:1, props:{}, settings:{}, children:[{id:"X", block:"core/accordion-item", version:1, props:{}, settings:{}, children:[]},{id:"Y", block:"core/accordion-item", version:1, props:{}, settings:{}, children:[]}]};
    reset([acc2, {id:"sec1", block:"core/section", version:1, props:{}, settings:{}, children:[]}]);
    assert.equal(moveNode({nodeId:"X", parentId:"sec1", index:0}).ok,false);
    assert.equal(moveNode({nodeId:"X", parentId:null, index:0}).ok,false);
  });

  it("67 destination max cross reject, same reorder allow", ()=>{
    const acc={id:"acc", block:"core/accordion", version:1, props:{}, settings:{}, children:[
      {id:"a1",block:"core/accordion-item",version:1,props:{},settings:{},children:[]},
      {id:"a2",block:"core/accordion-item",version:1,props:{},settings:{},children:[]},
      {id:"a3",block:"core/accordion-item",version:1,props:{},settings:{},children:[]}
    ]};
    reset([acc, {id:"outside", block:"core/accordion-item", version:1, props:{}, settings:{}, children:[]}]);
    assert.equal(moveNode({nodeId:"outside", parentId:"acc", index:3}).ok,false);
    const r2=moveNode({nodeId:"a1", parentId:"acc", index:3});
    assert.equal(r2.ok,true);
  });

  it("68 allowed blocks", ()=>{
    const acc={id:"acc", block:"core/accordion", version:1, props:{}, settings:{}, children:[]};
    reset([acc, mk("core/heading","h"), mk("core/accordion-item","ai")]);
    assert.equal(moveNode({nodeId:"h", parentId:"acc", index:0}).ok,false);
    assert.equal(moveNode({nodeId:"ai", parentId:"acc", index:0}).ok,true);
  });

  it("69 stable IDs preserved for subtree", ()=>{
    const child={id:"child1", block:"core/heading", version:1, props:{a:"x"}, settings:{}, children:[{id:"grand", block:"core/text", version:1, props:{t:"hi"}, settings:{}, children:[]}]};
    const A={id:"A", block:"core/section", version:1, props:{}, settings:{}, children:[child]};
    reset([A, mk("core/section","B")]);
    const beforeIds=collectIds(state.document.nodes).sort().join(",");
    const res=moveNode({nodeId:"child1", parentId:"B", index:0});
    assert.equal(res.ok,true);
    const afterIds=collectIds(state.document.nodes).sort().join(",");
    assert.equal(beforeIds,afterIds);
    assert.equal(state.document.nodes[1].children[0].id,"child1");
    assert.equal(state.document.nodes[1].children[0].children[0].id,"grand");
  });

  it("70 props/settings/RichText preserved", ()=>{
    const rich={version:1, content:[{text:"hello", marks:[{type:"bold"}]}, {text:" world", marks:[{type:"link", href:"https://a"}]}]};
    const node={id:"t1", block:"core/heading", version:1, props:{text:rich}, settings:{align:"center"}, children:[{id:"c1", block:"core/text", version:1, props:{text:"x"}, settings:{}, children:[]}]};
    reset([{id:"sec", block:"core/section", version:1, props:{}, settings:{}, children:[node]}, mk("core/section","sec2")]);
    const beforeProps=JSON.stringify(node.props);
    const res=moveNode({nodeId:"t1", parentId:"sec2", index:0});
    assert.equal(res.ok,true);
    const moved=findDocumentNode("t1");
    assert.equal(JSON.stringify(moved.props), beforeProps);
    assert.equal(moved.settings.align,"center");
    assert.equal(moved.children[0].id,"c1");
  });

  it("71 unknown parent rejected, no teleport", ()=>{
    reset([mk("core/heading","A")]);
    const r=moveNode({nodeId:"A", parentId:"nope", index:0});
    assert.equal(r.ok,false);
    assert.match(r.reason,/Parent not found/);
    assert.equal(state.document.nodes[0].id,"A");
  });

  it("hidden distinction: existing hidden block can be reordered, but not inserted", ()=>{
    const hiddenNode={id:"hid", block:"core/hidden-block", version:1, props:{}, settings:{}, children:[]};
    const other=mk("core/heading","oth");
    reset([hiddenNode, other]);
    const res=moveNode({nodeId:"hid", parentId:null, index:2});
    assert.equal(res.ok,true);
    const hiddenDef = catalog.find(c=>c.block==="core/hidden-block");
    const can = canInsert(null, hiddenDef, 0);
    assert.equal(can.ok,false);
  });

  it("atomic mutation no duplication", ()=>{
    const A=mk("core/heading","A"), B=mk("core/heading","B"), C=mk("core/heading","C");
    reset([A,B,C]);
    const beforeCount=countNodes(state.document.nodes);
    moveNode({nodeId:"B", parentId:null, index:0});
    assert.equal(countNodes(state.document.nodes), beforeCount);
  });

  it("findSource returns correct shape", ()=>{
    const sec={id:"sec", block:"core/section", version:1, props:{}, settings:{}, children:[mk("core/heading","h1"), mk("core/text","t1")]};
    reset([sec]);
    const src=findSource("t1");
    assert.equal(src.parentId,"sec");
    assert.equal(src.index,1);
    assert.equal(src.node.id,"t1");
    const rootSrc=findSource("sec");
    assert.equal(rootSrc.parentId,null);
    assert.equal(rootSrc.parent,null);
  });

  it("selection preserved after move", ()=>{
    const A=mk("core/heading","A"), B=mk("core/heading","B");
    reset([A,B]);
    state.selection={nodeId:"B", instanceKey:"k", editable:true};
    const res=moveNode({nodeId:"B", parentId:null, index:0});
    assert.equal(res.ok,true);
    assert.equal(state.selection.nodeId,"B");
  });

  it("subtree integrity - move Accordion with nested items between sections (M6 §12)", ()=>{
    const acc={id:"acc1", block:"core/accordion", version:1, props:{title:"A"}, settings:{color:"blue"}, children:[
      {id:"itemA", block:"core/accordion-item", version:1, props:{title:"IA"}, settings:{open:true}, children:[{id:"textA", block:"core/text", version:1, props:{text:"Text A"}, settings:{}, children:[]}]},
      {id:"itemB", block:"core/accordion-item", version:1, props:{title:"IB"}, settings:{open:false}, children:[{id:"textB", block:"core/text", version:1, props:{text:"Text B"}, settings:{}, children:[]}]}
    ]};
    const secA={id:"secA", block:"core/section", version:1, props:{}, settings:{}, children:[acc]};
    const secB={id:"secB", block:"core/section", version:1, props:{}, settings:{}, children:[]};
    reset([secA, secB]);
    // capture original props
    const origProps=JSON.stringify(acc.props);
    const origSettings=JSON.stringify(acc.settings);
    const itemAProps=JSON.stringify(acc.children[0].props);
    const itemBProps=JSON.stringify(acc.children[1].props);
    const res=moveNode({nodeId:"acc1", parentId:"secB", index:0});
    assert.equal(res.ok,true);
    // exact structure
    assert.equal(state.document.nodes.length,2);
    assert.equal(state.document.nodes[0].id,"secA");
    assert.equal(state.document.nodes[0].children.length,0);
    assert.equal(state.document.nodes[1].id,"secB");
    assert.equal(state.document.nodes[1].children.length,1);
    const moved=findDocumentNode("acc1");
    assert.ok(moved);
    assert.equal(moved.id,"acc1");
    assert.equal(moved.children.length,2);
    assert.equal(moved.children[0].id,"itemA");
    assert.equal(moved.children[1].id,"itemB");
    assert.equal(moved.children[0].children[0].id,"textA");
    assert.equal(moved.children[1].children[0].id,"textB");
    // shallow id checks
    const ids=collectIds(state.document.nodes);
    assert.equal(ids.length,new Set(ids).size,"no duplicate IDs");
    assert.equal(ids.includes("acc1"),true);
    assert.equal(ids.includes("itemA"),true);
    assert.equal(ids.includes("itemB"),true);
    assert.equal(ids.includes("textA"),true);
    assert.equal(ids.includes("textB"),true);
    // old parent contains none of subtree
    assert.equal(findDocumentNode("secA").children.some(c=>c.id==="acc1"),false);
    assert.equal(findDocumentNode("secA").children.some(c=>c.id==="itemA"),false);
    // props/settings unchanged
    assert.equal(JSON.stringify(moved.props),origProps);
    assert.equal(JSON.stringify(moved.settings),origSettings);
    assert.equal(JSON.stringify(moved.children[0].props),itemAProps);
    assert.equal(JSON.stringify(moved.children[1].props),itemBProps);
    assertTreeIntegrity(state.document, { definitionForBlock });
  });

  it("subtree integrity - move whole Section containing nested Accordion at root (M6 §12)", ()=>{
    const acc={id:"acc1", block:"core/accordion", version:1, props:{}, settings:{}, children:[
      {id:"itemA", block:"core/accordion-item", version:1, props:{}, settings:{}, children:[{id:"textA", block:"core/text", version:1, props:{text:"A"}, settings:{}, children:[]}]},
      {id:"itemB", block:"core/accordion-item", version:1, props:{}, settings:{}, children:[{id:"textB", block:"core/text", version:1, props:{text:"B"}, settings:{}, children:[]}]}
    ]};
    const secA={id:"secA", block:"core/section", version:1, props:{layout:"wide"}, settings:{}, children:[acc]};
    const secB={id:"secB", block:"core/section", version:1, props:{}, settings:{}, children:[mk("core/text","t0")]};
    const other=mk("core/heading","other");
    reset([secA, secB, other]);
    const beforeIds=collectIds(state.document.nodes).sort().join(",");
    const res=moveNode({nodeId:"secA", parentId:null, index:3});
    assert.equal(res.ok,true);
    assert.deepEqual(state.document.nodes.map(n=>n.id), ["secB","other","secA"]);
    const afterIds=collectIds(state.document.nodes).sort().join(",");
    assert.equal(beforeIds,afterIds);
    const movedSec=findDocumentNode("secA");
    assert.equal(movedSec.children[0].id,"acc1");
    assert.equal(movedSec.children[0].children[0].id,"itemA");
    assert.equal(movedSec.children[0].children[1].id,"itemB");
    assertTreeIntegrity(state.document, { definitionForBlock });
  });

  it("placement respects accordion-item constraints via integrity helper", ()=>{
    reset([]);
    assertTreeIntegrity(state.document, { definitionForBlock });
    const acc={id:"acc", block:"core/accordion", version:1, props:{}, settings:{}, children:[
      {id:"a1", block:"core/accordion-item", version:1, props:{}, settings:{}, children:[]},
      {id:"a2", block:"core/accordion-item", version:1, props:{}, settings:{}, children:[]}
    ]};
    const sec={id:"sec", block:"core/section", version:1, props:{}, settings:{}, children:[]};
    reset([acc, sec]);
    assertTreeIntegrity(state.document, { definitionForBlock });
    // Item cannot move to root/section/stack
    assert.equal(canMove("a1", null, 1).ok,false);
    assert.equal(canMove("a1", "sec", 0).ok,false);
    // But can move inside same (to end) or other accordion
    // a1 at 0, moving to 1 is no-op (already there); moving to 2 is reorder to end
    assert.equal(canMove("a1", "acc", 2).ok,true);
    const accB={id:"accB", block:"core/accordion", version:1, props:{}, settings:{}, children:[{id:"b1", block:"core/accordion-item", version:1, props:{}, settings:{}, children:[]} ]};
    reset([acc, accB]);
    const mv=moveNode({nodeId:"a1", parentId:"accB", index:1});
    assert.equal(mv.ok,true);
    assertTreeIntegrity(state.document, { definitionForBlock });
  });
});
