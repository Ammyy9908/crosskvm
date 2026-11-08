const fs=require('fs');
const path=require('path');
const {spawn}=require('child_process');
// Long-lived STA OLE target. Only the configured edge is exposed during a
// physical Explorer drag; the helper never reads the clipboard/selection.
function createWindowsDragSource(onError=()=>{}) {
 const exe=path.join(process.env.SystemRoot||'C:\\Windows','System32','WindowsPowerShell','v1.0','powershell.exe');
 const script='[Console]::InputEncoding=New-Object Text.UTF8Encoding; [Console]::OutputEncoding=New-Object Text.UTF8Encoding; $ErrorActionPreference="Stop"; $s=[Console]::ReadLine() | ConvertFrom-Json; Add-Type -TypeDefinition $s -ReferencedAssemblies System.Windows.Forms,System.Drawing,System.Web.Extensions; [CrossKVM.EdgeCapture]::Run()';
 const child=spawn(exe,['-NoProfile','-NonInteractive','-STA','-Command',script],{windowsHide:true,stdio:['pipe','pipe','pipe']});
 let state={change:0,down:false,files:[]},buffer='',errors='',closed=false,lastSide,gesture=0,physicalDown=false;
 child.stdin.on('error',()=>{});
 child.stdout.on('data',bytes=>{
  buffer+=bytes.toString();let n;
  while((n=buffer.indexOf('\n'))>=0){
   const line=buffer.slice(0,n);buffer=buffer.slice(n+1);
   try {const next=JSON.parse(line);if(next.gesture===gesture&&Number.isInteger(next.change)&&typeof next.down==='boolean'&&Array.isArray(next.files)&&next.files.length<=100&&next.files.every(f=>typeof f==='string'))state={...next,down:physicalDown};}catch(_){}
  }
  if(buffer.length>1048576){buffer='';onError('Explorer drag data exceeded the limit');}
 });
 child.stderr.on('data',b=>{errors=(errors+b.toString()).slice(-2000);});
 child.on('error',err=>onError('Explorer drag capture unavailable: '+err.message));
 child.on('exit',()=>{state={change:state.change,down:false,files:[]};if(!closed)onError('Explorer drag capture stopped: '+errors.slice(-500));});
 child.stdin.write(JSON.stringify(fs.readFileSync(path.join(__dirname,'windows-drag-source.cs'),'utf8'))+'\n');
 const command=s=>{if(!closed&&!child.killed)child.stdin.write(s+'\n');};
 return {
  readDrag:()=>state,
  physicalButton:down=>{physicalDown=down;if(down)gesture++;state={...state,down,files:[]};command(down?'down:'+gesture:'up');},
  cancelFinder:()=>command('cancel'),
  configure:status=>{const side=status.kvmActive&&status.edgeDragAvailable?status.peerSide:'none';if(side!==lastSide){lastSide=side;command(['left','right','top','bottom'].includes(side)?side:'none');}},
  close:()=>{command('close');closed=true;setTimeout(()=>child.kill(),1000).unref();}
 };
}
module.exports={createWindowsDragSource};
