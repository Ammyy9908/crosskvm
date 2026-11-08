const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('fs/promises');
const os=require('os');
const path=require('path');
const {EdgeDrag}=require('../src/main/edge-drag');
const {randomUUID}=require('crypto');

test('drop and return to source do not cancel an unfinished background transfer',async()=>{
 const root=await fs.mkdtemp(path.join(os.tmpdir(),'crosskvm-promise-'));
 const file=path.join(root,'hello.txt');await fs.writeFile(file,'hello');
 let state={change:0,down:false,files:[]},finishTransfer,notifyOffer;
 const packets=[];let cancelled;
 const offered=new Promise(r=>notifyOffer=r);
 const edge=new EdgeDrag({native:{readDrag:()=>state,cancelFinder:()=>{}},platform:'win32',
 send:async p=>{packets.push(p);if(p.kind==='drag_start')notifyOffer(p);},status:async()=>({edgeDragAvailable:true}),
 transfer:{sendFiles:async(files,options)=>{cancelled=options.cancelled;await new Promise(r=>finishTransfer=r);}},notify:()=>{}});
 try{
  state={change:1,down:true,files:[file]};const sending=edge.control(true);
  const offer=await offered;await new Promise(r=>setImmediate(r));
  assert.deepEqual(offer.files,[{name:'hello.txt',size:5}]);
  await edge.packet({kind:'drag_ready',id:offer.id});
  edge.release(true);await edge.control(false);
  assert.equal(cancelled(),false);assert.equal(packets.at(-1).kind,'drag_release');
  finishTransfer();await sending;assert.ok(edge.source);
  await edge.packet({kind:'drag_result',id:offer.id,result:'dropped'});assert.equal(edge.source,null);
 }finally{finishTransfer?.();edge.close();await fs.rm(root,{recursive:true,force:true});}
});

for(const platform of ['darwin','win32'])test(platform+' accepts a drop before bytes arrive and delivers each promise',async()=>{
 const packets=[],provided=[];let drops=0,cancels=0,finish;
 const edge=new EdgeDrag({platform,send:async p=>packets.push(p),notify:()=>{},transfer:{},status:async()=>({}),
 startDrag:(files,result)=>{assert.deepEqual(files,[{name:'hello.txt',size:5}]);finish=result;return{ready:Promise.resolve(),provide:(i,p)=>provided.push([i,p]),drop:()=>drops++,cancel:()=>cancels++};}});
 try{
  const id=randomUUID();await edge.packet({kind:'drag_start',id,files:[{name:'hello.txt',size:5}]});
  assert.equal(packets.at(-1).kind,'drag_ready');
  await edge.packet({kind:'drag_release',id});assert.equal(drops,1);assert.equal(provided.length,0);
  assert.throws(()=>edge.stagedFile('other','/cache/file',0),/matching/);
  assert.throws(()=>edge.stagedFile(id,'/cache/file',2),/matching/);
  edge.stagedFile(id,'/cache/file',0);assert.deepEqual(provided,[[0,'/cache/file']]);
  assert.equal(packets.filter(p=>p.kind==='drag_result').length,0);
  finish('dropped');assert.equal(packets.at(-1).result,'dropped');
  await edge.packet({kind:'drag_start',id:randomUUID(),files:[{name:'hello.txt',size:5}]});edge.reset();assert.equal(cancels,1);
 }finally{edge.close();}
});

test('untrusted promise metadata cannot escape a destination filename',async()=>{
 let starts=0;const packets=[];
 const edge=new EdgeDrag({platform:'darwin',send:async p=>packets.push(p),notify:()=>{},transfer:{},status:async()=>({}),startDrag:()=>{starts++;}});
 try{for(const files of [[{name:'../bad',size:1}],[{name:'ok',size:-1}],[],[{name:'ok',size:2**40}]])await edge.packet({kind:'drag_start',id:randomUUID(),files});assert.equal(starts,0);assert.equal(packets.length,4);}
 finally{edge.close();}
});

test('Mac native adapter waits for AppKit readiness and completes once',async()=>{
 const {startMacDrag}=require('../src/main/edge-drag');let state=1,begins=0,drops=0,results=[];
 const drag=startMacDrag({begin:()=>begins++,status:()=>state,drop:()=>{drops++;state=3;},cancel:()=>{state=4;}},['/cache/file'],r=>results.push(r));
 state=2;await drag.ready;assert.equal(begins,1);drag.drop();
 await new Promise(r=>setTimeout(r,60));
 assert.equal(drops,1);assert.deepEqual(results,['dropped']);drag.cancel();assert.equal(results.length,1);
});

test('Mac native completion between status polls does not report startup failure',async()=>{
 const {startMacDrag}=require('../src/main/edge-drag');const results=[];
 const drag=startMacDrag({begin:()=>{},status:()=>({state:3,started:true,error:''}),drop:()=>{},cancel:()=>{}},['/cache/file'],r=>results.push(r));
 await drag.ready;assert.deepEqual(results,['dropped']);
});

test('Mac pending drag preserves the early-release cancellation reason',async()=>{
 const {startMacDrag}=require('../src/main/edge-drag');
 const drag=startMacDrag({begin:()=>{},status:()=>({state:1,started:false,error:''}),drop:()=>{},cancel:()=>{}},['/cache/file'],()=>{});
 const rejected=assert.rejects(drag.ready,/Mouse released before/);
 drag.cancel('Mouse released before the destination drag was ready');await rejected;
});
