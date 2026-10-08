const test=require('node:test');
const assert=require('node:assert/strict');
const {imageClipboard,validPNG}=require('../src/main/clipboard-images');
const png=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a6V8AAAAASUVORK5CYII=','base64');
test('Electron ClipboardItem image adapter preserves PNG and enforces limits',async()=>{
 let written;
 class Item {constructor(data){this.data=data;}}
 const adapter=imageClipboard({
  read:async()=>[{types:['image/png'],getType:async()=>new Blob([png])}],
  write:async value=>{written=value;}
 },Item);
 assert.equal(validPNG(png),true);
 const snapshot=await adapter.readImageSnapshot();
 assert.equal(snapshot.png,png.toString('base64'));
 await adapter.writePNG(snapshot.png);
 assert.deepEqual(Buffer.from(await written[0].data['image/png'].arrayBuffer()),png);
 await assert.rejects(()=>adapter.writePNG('bad data'));
 const giant=Buffer.from(png);giant.writeUInt32BE(100000,16);giant.writeUInt32BE(100000,20);
 assert.equal(validPNG(giant),false);
});
test('native Mac image data takes priority over filename text',async()=>{
 let reads=0;
 const adapter=imageClipboard({read:async()=>{reads++;return[];}},null,null,{readPNG:()=>png});
 assert.equal((await adapter.readImageSnapshot()).png,png.toString('base64'));
 assert.equal(reads,0);
});
test('file references and native conversion failures block filename fallback',async()=>{
 const files=imageClipboard({read:async()=>[{types:['text/plain','electron application/osclipboard;format="public.file-url"']}]});
 assert.match((await files.readImageSnapshot()).error,/file was copied/i);
 const large=imageClipboard({read:async()=>{throw Error('should not fall back');}},null,null,{readPNG:()=>{throw Error('Image too large');}});
 assert.equal((await large.readImageSnapshot()).error,'Image too large');
});
test('JPEG clipboard content is normalized to PNG',async()=>{
 const adapter=imageClipboard({read:async()=>[{types:['image/jpeg'],getType:async()=>new Blob(['jpeg'])}]},null,{
 createFromBuffer:()=>({getSize:()=>({width:1,height:1}),isEmpty:()=>false,toPNG:()=>png})
 });
 assert.equal((await adapter.readImageSnapshot()).png,png.toString('base64'));
});
