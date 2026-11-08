const fs = require('fs');
const path = require('path');
const {spawn} = require('child_process');
const {safeName}=require('./file-transfer');
const {randomUUID} = require('crypto');

function startWindowsDrag(files, result) {
  const script = '[Console]::InputEncoding=New-Object Text.UTF8Encoding; [Console]::OutputEncoding=New-Object Text.UTF8Encoding; $ErrorActionPreference="Stop"; $r=[Console]::ReadLine() | ConvertFrom-Json; Add-Type -TypeDefinition $r.source -ReferencedAssemblies System.Windows.Forms,System.Web.Extensions; [CrossKVM.NativeDrag]::Run([string[]]$r.names,[long[]]$r.sizes)';
  const exe = path.join(process.env.SystemRoot || 'C:\\Windows','System32','WindowsPowerShell','v1.0','powershell.exe');
  const child = spawn(exe,['-NoProfile','-NonInteractive','-STA','-Command',script],{windowsHide:true,stdio:['pipe','pipe','pipe']});
  let settled=false,ready=false,buffer='',errors='',outcome='cancelled';
  let resolveReady,rejectReady;
  const pending=new Promise((resolve,reject)=>{resolveReady=resolve;rejectReady=reject;});
  const timer=setTimeout(()=>{child.kill();if(!ready)rejectReady(new Error('Windows drag startup timed out'));},20000);
  child.stdin.on('error',()=>{});
  child.stdout.on('data',chunk=>{
    buffer+=chunk.toString();
    let index;
    while((index=buffer.indexOf('\n'))>=0){
      const line=buffer.slice(0,index).trim();buffer=buffer.slice(index+1);
      if(line==='ready'){ready=true;clearTimeout(timer);resolveReady();}
      if(line==='dropped'||line==='cancelled')outcome=line;
    }
    if(buffer.length>8192)buffer='';
  });
  child.stderr.on('data',chunk=>{errors=(errors+chunk.toString()).slice(-2000);});
  const finish=err=>{
    if(settled)return;settled=true;clearTimeout(timer);
    if(!ready)rejectReady(err || new Error('Windows could not start native dragging: '+errors.slice(-400)));
    result(err ? 'failed' : outcome,err?.message);
  };
  child.on('error',finish);
  child.on('exit',code=>finish(code ? new Error('Native drag helper exited') : null));
  child.stdin.write(JSON.stringify({source:fs.readFileSync(path.join(__dirname,'windows-drag.cs'),'utf8'),names:files.map(f=>f.name),sizes:files.map(f=>f.size)})+'\n');
  return {
    ready:pending,
    provide:(index,file)=>{if(!settled)child.stdin.write(JSON.stringify({index,path:file})+'\n');},
    drop:()=>{if(!settled)child.stdin.write('drop\n');},
    cancel:()=>{if(!settled){child.stdin.write('cancel\n');setTimeout(()=>{if(!settled)child.kill();},1000).unref();}}
  };
}

function startMacDrag(native,files,result) {
 let finished=false,resolveReady,rejectReady;
 const ready=new Promise((resolve,reject)=>{resolveReady=resolve;rejectReady=reject;});
 let started=false,timer;
 const finish=(outcome,error)=>{
  if(finished)return;finished=true;clearInterval(timer);
  if(!started)rejectReady(error||new Error('Finder drag could not start'));
  result(outcome,error?.message);
 };
 const startupDeadline=Date.now()+10000;
 const deadline=Date.now()+30*60*1000;
 try {
  native.begin(files);
  timer=setInterval(()=>{
   const snapshot=native.status();
   const state=typeof snapshot==='number'?snapshot:snapshot.state;
   if((state===2||state===3||snapshot.started)&&!started){started=true;resolveReady();}
   if(state===3||state===4||state===5)finish(state===3?'dropped':state===5?'failed':'cancelled',snapshot.error?new Error(snapshot.error):undefined);
   else if(Date.now()>deadline||(!started&&Date.now()>startupDeadline)){native.cancel();finish('cancelled',new Error('Finder drag timed out'));}
  },25);
 }catch(err){queueMicrotask(()=>finish('failed',err));}
 return {ready,provide:(index,file)=>native.provide(index,file),drop:()=>{if(!finished)native.drop();},cancel:(reason)=>{if(!finished){native.cancel();finish('cancelled',reason?new Error(reason):undefined);}}};
}

class EdgeDrag {
  constructor({native,send,status,transfer,notify,platform=process.platform,startDrag=startWindowsDrag}) {
    this.native=native;this.send=send;this.status=status;this.transfer=transfer;this.notify=notify;
    this.platform=platform;this.startDrag=startDrag;this.source=null;this.target=null;this.staged=null;
    this.change=native ? native.readDrag().change : null;this.candidate=null;this.remote=false;this.pendingChange=null;
    this.timer=native ? setInterval(()=>this.sample(),80) : null;
    this.configuring=false;
    this.configTimer=native?.configure ? setInterval(async()=>{
      if(this.configuring)return;this.configuring=true;
      try{native.configure(await this.status());}catch(_){native.configure({});}
      finally{this.configuring=false;}
    },1000) : null;
  }
  notice(message){this.notify(message);}
  sample(){
    if(!this.native)return;
    try{
      const state=this.native.readDrag();
      if(!state.down){
        if(this.source)this.release();
        this.candidate=null;this.pendingChange=null;this.change=state.change;return;
      }
      if(state.change!==this.change){
        this.change=state.change;this.pendingChange=state.change;this.candidate=null;
      }
      // Finder can publish the drag pasteboard before the file URLs resolve.
      // Retry that fresh change while held instead of discarding it forever.
      if(this.pendingChange===state.change && state.files.length){
        this.candidate=state.files;
        if(this.remote && !this.source)setImmediate(()=>{if(this.remote && !this.source && this.candidate)this.control(true).catch(err=>this.notice(err.message));});
      }
    }catch(_){this.candidate=null;}
  }
  async control(remote){
    this.remote=remote;
    if(!remote){if(!this.source?.released)this.cancelSource();return;}
    if(!this.native||this.source||this.transfer.busy)return;
    this.sample();
    const files=this.candidate;this.candidate=null;
    if(files)this.pendingChange=null;
    if(!files){
      if(this.native.readDrag().down)this.notice('Waiting for file drag details — keep holding; Escape cancels');
      return;
    }
    const session={id:randomUUID(),ready:false};
    this.source=session;
    try{
      const status=await this.status();
      if(this.source!==session || !this.remote)return;
      if(!status.edgeDragAvailable)throw new Error('Update both apps to use screen-edge dragging');
      const manifest=[];
      const names=new Set();
      for(const file of files){
        const stat=await fs.promises.stat(file);
        if(!stat.isFile()||stat.size>1024*1024*1024)throw new Error('Choose regular files up to 1 GiB');
        let name=safeName(path.basename(file));const base=path.parse(name);let n=0;
        while(names.has(name.toLowerCase()))name=base.name+' ('+(++n)+')'+base.ext;
        names.add(name.toLowerCase());manifest.push({name,size:stat.size});
      }
      if(this.source!==session)return;
      this.native.cancelFinder();
      this.notice('Drag into the destination folder; copying continues after you drop');
      session.timer=setTimeout(()=>{if(this.source===session)this.cancelSource();},30*60*1000);
      await this.send({kind:'drag_start',id:session.id,files:manifest});
      if(this.source!==session)return;
      await this.transfer.sendFiles(files,{dragId:session.id,cancelled:()=>this.source!==session});
    }catch(err){
      if(this.source===session){this.notice(err.message);this.cancelSource();}
    }
  }
  release(physical=false){
    const source=this.source;if(!source)return;
    // Finder may synthesize a release while cancelling its original drag.
    if(!(physical && this.platform==='win32') && this.native && this.native.readDrag().down)return;
    if(!source.ready){this.notice('Drag cancelled before the destination accepted the drag; try again when the copy cursor appears');this.cancelSource('early-release');return;}
    if(source.released)return;
    source.released=true;
    this.notice('Copying files in the background…');
    this.send({kind:'drag_release',id:source.id}).catch(()=>this.cancelSource());
  }
  cancelSource(reason='cancelled'){
    const source=this.source;this.source=null;
    if(source){clearTimeout(source.timer);this.send({kind:'drag_cancel',id:source.id,reason}).catch(()=>{});}
  }
  stagedFile(id,file,index){
    const target=this.target;
    if(!target||target.id!==id||!Number.isInteger(index)||index<0||index>=target.count)throw new Error('No matching promised file');
    target.helper.provide(index,file);
  }
  async packet(p){
    if(!p.kind?.startsWith('drag_'))return false;
    if(p.kind==='drag_ready' && this.source?.id===p.id){
      this.source.ready=true;this.notice('Destination ready — drop now; copying continues in the background');return true;
    }
    if(p.kind==='drag_result' && this.source?.id===p.id){
      clearTimeout(this.source.timer);this.source=null;
      this.notice(p.result==='dropped'?'Copy complete':p.error?'Copy failed: '+p.error:'File drag cancelled');return true;
    }
    if(p.kind==='drag_cancel'){
      if(this.target?.id===p.id){this.target.helper.cancel(p.reason==='early-release'?'Mouse released before the destination drag was ready — keep holding until the copy cursor appears':'Drag cancelled by sender');this.target=null;}
      if(this.staged?.id===p.id){clearTimeout(this.staged.timer);this.staged=null;}
      return true;
    }
    if(p.kind==='drag_release'){
      if(this.target?.id===p.id && this.target.ready){this.target.released=true;this.notice('Copying files to the dropped destination…');this.target.helper.drop();}
      return true;
    }
    if(p.kind==='drag_start'){
      const valid=Array.isArray(p.files)&&p.files.length>0&&p.files.length<=100&&p.files.every(f=>f&&typeof f.name==='string'&&f.name.length<=180&&safeName(f.name)===f.name&&Number.isSafeInteger(f.size)&&f.size>=0&&f.size<=1024*1024*1024);
      if(!['win32','darwin'].includes(this.platform)||this.target||!valid){
        await this.send({kind:'drag_result',id:p.id,result:'cancelled'});return true;
      }
      const target={id:p.id,ready:false,count:p.files.length};this.target=target;
      try{
        target.helper=this.startDrag(p.files,(outcome,error)=>{
          if(this.target!==target)return;
          this.target=null;
          this.notice(outcome==='dropped'?'Copy complete':error?'Copy failed: '+error:'File drag cancelled');
          this.send({kind:'drag_result',id:p.id,result:outcome,...(error?{error}: {})}).catch(()=>{});
        });
        await target.helper.ready;
        if(this.target!==target){target.helper.cancel();return true;}
        target.ready=true;
        await this.send({kind:'drag_ready',id:p.id});
      }catch(err){
        if(this.target===target)this.target=null;
        target.helper?.cancel();
        this.notice(err.message);
        await this.send({kind:'drag_result',id:p.id,result:'cancelled'}).catch(()=>{});
      }
      return true;
    }
    return true;
  }
  reset(){
    this.cancelSource();this.candidate=null;this.pendingChange=null;this.remote=false;
    this.target?.helper?.cancel();this.target=null;
    if(this.staged)clearTimeout(this.staged.timer);this.staged=null;
  }
  close(){this.reset();clearInterval(this.timer);clearInterval(this.configTimer);this.native?.close?.();}
}
module.exports={EdgeDrag,startWindowsDrag,startMacDrag};
