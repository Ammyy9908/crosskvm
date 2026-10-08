const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('fs/promises');
const path=require('path');
const os=require('os');
const {randomUUID}=require('crypto');
const {FileTransfer,safeName}=require('../src/main/file-transfer');
test('chunked disk transfer preserves bytes, empty files and existing names',async()=>{
 const root=await fs.mkdtemp(path.join(os.tmpdir(),'crosskvm-files-'));
 let a,b;
 const notices=[];
 a=new FileTransfer({downloads:path.join(root,'a'),notify:p=>notices.push(p),send:async p=>{void b.receive(p);}});
 b=new FileTransfer({downloads:path.join(root,'b'),notify:p=>notices.push(p),send:async p=>{void a.receive(p);}});
 try {
  const source=path.join(root,'photo.jpg'),bytes=Buffer.alloc(200000);
  for(let i=0;i<bytes.length;i++)bytes[i]=i%251;
  await fs.writeFile(source,bytes);
  await a.sendFiles([source]); await a.sendFiles([source]);
  assert.deepEqual(await fs.readFile(path.join(root,'b','photo.jpg')),bytes);
  assert.deepEqual(await fs.readFile(path.join(root,'b','photo (1).jpg')),bytes);
  await fs.writeFile(path.join(root,'empty'),Buffer.alloc(0));
  await a.sendFiles([path.join(root,'empty')]);
  assert.equal((await fs.stat(path.join(root,'b','empty'))).size,0);
  assert.equal((await fs.readdir(path.join(root,'b'))).some(n=>n.endsWith('.part')),false);
  assert.equal(notices.filter(p=>p.state==='complete').length,6);
 }finally{await a.reset();await b.reset();await fs.rm(root,{recursive:true,force:true});}
});
test('malformed chunks, checksum mismatch and disconnect remove partial files',async()=>{
 const root=await fs.mkdtemp(path.join(os.tmpdir(),'crosskvm-files-'));
 const replies=[];
 const receiver=new FileTransfer({downloads:root,notify:()=>{},send:async p=>{replies.push(p);}});
 try{
  for(const mode of ['chunk','checksum','disconnect']){
   const id=randomUUID();
   await receiver.receive({kind:'begin',id,step:0,name:'../unsafe.txt',size:1});
   if(mode==='chunk') await receiver.receive({kind:'chunk',id,step:1,offset:0,data:'!!!'});
   if(mode==='checksum'){
    await receiver.receive({kind:'chunk',id,step:1,offset:0,data:'YQ=='});
    await receiver.receive({kind:'end',id,step:2,sha256:'wrong'});
   }
   if(mode==='disconnect')await receiver.reset();
   assert.deepEqual(await fs.readdir(root),[]);
  }
  assert.equal(replies.filter(p=>p.kind==='error').length,2);
 }finally{await receiver.reset();await fs.rm(root,{recursive:true,force:true});}
});
test('sender fails on timeout and names cannot escape Downloads',async()=>{
 assert.equal(safeName('../../foo.txt'),'foo.txt');
 assert.equal(safeName('C:\\temp\\CON.txt'),'_CON.txt');
 const root=await fs.mkdtemp(path.join(os.tmpdir(),'crosskvm-files-'));
 const sender=new FileTransfer({downloads:root,notify:()=>{},send:async()=>{},timeout:20});
 try{
  const p=path.join(root,'file');await fs.writeFile(p,'a');
  await assert.rejects(sender.sendFiles([p]),/timed out/);
  assert.equal(sender.busy,false);
 }finally{await sender.reset();await fs.rm(root,{recursive:true,force:true});}
});
