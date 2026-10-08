const test = require('node:test');
const assert = require('node:assert/strict');
const ClipboardSync = require('../src/main/clipboard-sync');
for (const asyncAPI of [false, true]) {
 test(`clipboard round trip and no echo (async=${asyncAPI})`, async () => {
  let text='preexisting';const sent=[];
  const native={readText:()=>asyncAPI?Promise.resolve(text):text,writeText:value=>{text=value;return asyncAPI?Promise.resolve():undefined;}};
  const sync=new ClipboardSync(native,t=>sent.push(t));
  await sync.setEnabled(true);await sync.poll();assert.deepEqual(sent,[]);
  text='Hello\nनमस्ते 👋';await sync.poll();assert.deepEqual(sent,[text]);
  await sync.receive('Remote\n你好');await sync.poll();assert.equal(text,'Remote\n你好');assert.equal(sent.length,1);
  await sync.setEnabled(false);await sync.receive('ignored');assert.equal(text,'Remote\n你好');
  text='offline secret';await sync.setEnabled(true);await sync.poll();assert.equal(sent.length,1);
  text='é'.repeat(40000);await sync.poll();assert.equal(sent.length,1);
  text='';await sync.poll();assert.equal(sent.length,1);
 });
}
test('disconnect during a pending native read does not publish old clipboard',async()=>{
 let resolve;const sent=[];let slow=false;
 const sync=new ClipboardSync({readText:()=>slow?new Promise(r=>resolve=r):'baseline',writeText:()=>{}},t=>sent.push(t));
 await sync.setEnabled(true);slow=true;const pending=sync.poll();await Promise.resolve();sync.setEnabled(false);resolve('late text');await pending;assert.deepEqual(sent,[]);
});
test('images: baseline, send, receive without echo, off/on and text fallback', async () => {
 let text = '', image = {key:'image:initial', png:'initial'};
 const sent=[], images=[];
 const sync = new ClipboardSync(
  {readText:()=>text, writeText:t=>{text=t; image=null;}},
  t=>sent.push(t),
  {readImageSnapshot:async()=>image, writePNG:async png=>{image={key:'image:'+png,png};}},
  png=>images.push(png));
 sync.setImagesEnabled(true);
 await sync.setEnabled(true);
 await sync.poll(); assert.deepEqual(images,[]);
 image={key:'image:new',png:'new'}; await sync.poll(); assert.deepEqual(images,['new']);
 await sync.receiveImage('remote'); await sync.poll(); assert.deepEqual(images,['new']);
 await sync.setEnabled(false); await sync.receiveImage('blocked'); assert.equal(image.png,'remote');
 image={key:'image:offline',png:'offline'};
 await sync.setEnabled(true); await sync.poll(); assert.deepEqual(images,['new']);
 await sync.receive('remote text'); await sync.poll(); assert.deepEqual(sent,[]);
 image=null; text='fresh text'; await sync.poll(); assert.deepEqual(sent,['fresh text']);
 sync.setImagesEnabled(false); await sync.receiveImage('unsupported'); assert.equal(image,null);
});
test('oversized image shows notice and leaves clipboard untouched', async()=>{
 const notices=[];let image=null;
 const sync=new ClipboardSync({readText:()=>'',writeText:()=>{}},()=>assert.fail('sent text'),
 {readImageSnapshot:()=>image},()=>assert.fail('sent image'),m=>notices.push(m));
 sync.setImagesEnabled(true);await sync.setEnabled(true);
 image={key:'large',error:'too large'};await sync.poll();await sync.poll();
 assert.deepEqual(notices,['too large']);
});
