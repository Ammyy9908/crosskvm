using System;
using System.IO;
using System.Threading;
using System.Runtime.InteropServices;
using System.Runtime.InteropServices.ComTypes;
using System.Windows.Forms;
using System.Web.Script.Serialization;
namespace CrossKVM {
 [ComVisible(true), Guid("00000121-0000-0000-C000-000000000046"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
 public interface IDropSource {
  [PreserveSig] int QueryContinueDrag([MarshalAs(UnmanagedType.Bool)] bool escape,uint keys);
  [PreserveSig] int GiveFeedback(uint effect);
 }
 [ComVisible(true), Guid("3D8B0590-F691-11D2-8EA9-006097DF5BD4"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
 public interface IDataObjectAsyncCapability {
  [PreserveSig] int SetAsyncMode([MarshalAs(UnmanagedType.Bool)] bool enabled);
  [PreserveSig] int GetAsyncMode([MarshalAs(UnmanagedType.Bool)] out bool enabled);
  [PreserveSig] int StartOperation(IBindCtx context);
  [PreserveSig] int InOperation([MarshalAs(UnmanagedType.Bool)] out bool active);
  [PreserveSig] int EndOperation(int result,IBindCtx context,uint effects);
 }
 public class PromisedFile {
  public string Name,Path;public long Size;public bool ReadComplete;public Exception Error;
  public readonly ManualResetEventSlim Ready=new ManualResetEventSlim(false);
  public FileStream Open() {
   if(!Ready.Wait(TimeSpan.FromMinutes(30)))throw new IOException("File transfer timed out");
   if(Error!=null)throw new IOException("File transfer cancelled",Error);
   var stream=new FileStream(Path,FileMode.Open,FileAccess.Read,FileShare.Read);
   if(stream.Length!=Size){stream.Dispose();throw new IOException("File size changed");}return stream;
  }
 }
 [ComVisible(true),ClassInterface(ClassInterfaceType.None)]
 public class PromiseStream : IStream {
  readonly PromisedFile file;FileStream stream;long position;
  public PromiseStream(PromisedFile file){this.file=file;}
  void Ensure(){if(stream==null){stream=file.Open();stream.Position=position;}}
  public void Read(byte[] buffer,int count,IntPtr read){Ensure();int n=stream.Read(buffer,0,count);position+=n;if(position>=file.Size)file.ReadComplete=true;if(read!=IntPtr.Zero)Marshal.WriteInt32(read,n);}
  public void Write(byte[] b,int n,IntPtr p){throw new NotSupportedException();}
  public void Seek(long offset,int origin,IntPtr newPosition){long next=origin==0?offset:origin==1?position+offset:file.Size+offset;if(next<0)throw new IOException("Invalid seek");position=next;if(stream!=null)stream.Position=next;if(newPosition!=IntPtr.Zero)Marshal.WriteInt64(newPosition,next);}
  public void SetSize(long size){throw new NotSupportedException();}
  public void CopyTo(IStream target,long count,IntPtr read,IntPtr written){
   Ensure();long total=0;byte[] bytes=new byte[65536];IntPtr actual=Marshal.AllocHGlobal(4);
   try{while(total<count){int n=stream.Read(bytes,0,(int)Math.Min(bytes.Length,count-total));if(n==0)break;target.Write(bytes,n,actual);if(Marshal.ReadInt32(actual)!=n)throw new IOException("Incomplete destination write");total+=n;position+=n;}if(position>=file.Size)file.ReadComplete=true;}
   finally{Marshal.FreeHGlobal(actual);if(read!=IntPtr.Zero)Marshal.WriteInt64(read,total);if(written!=IntPtr.Zero)Marshal.WriteInt64(written,total);}
  }
  public void Commit(int flags){}
  public void Revert(){throw new NotSupportedException();}
  public void LockRegion(long a,long b,int c){throw new NotSupportedException();}
  public void UnlockRegion(long a,long b,int c){throw new NotSupportedException();}
  public void Stat(out System.Runtime.InteropServices.ComTypes.STATSTG stat,int flags){stat=new System.Runtime.InteropServices.ComTypes.STATSTG();stat.type=2;stat.cbSize=file.Size;if((flags&1)==0)stat.pwcsName=file.Name;}
  public void Clone(out IStream clone){var copy=new PromiseStream(file);copy.position=position;clone=copy;}
  ~PromiseStream(){if(stream!=null)stream.Dispose();}
 }
 [ComVisible(true),ClassInterface(ClassInterfaceType.None)]
 public class Formats : IEnumFORMATETC {
  readonly FORMATETC[] formats;int index;
  public Formats(FORMATETC[] formats){this.formats=formats;}
  public int Next(int count,FORMATETC[] values,int[] fetched){int n=0;while(n<count&&index<formats.Length)values[n++]=formats[index++];if(fetched!=null&&fetched.Length>0)fetched[0]=n;return n==count?0:1;}
  public int Skip(int count){int old=index;index=Math.Min(index+count,formats.Length);return index-old==count?0:1;}
  public int Reset(){index=0;return 0;}
  public void Clone(out IEnumFORMATETC clone){var result=new Formats(formats);result.index=index;clone=result;}
 }
 [ComVisible(true),ClassInterface(ClassInterfaceType.None)]
 public class VirtualFiles : System.Runtime.InteropServices.ComTypes.IDataObject,IDataObjectAsyncCapability {
  [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern uint RegisterClipboardFormat(string format);
  [DllImport("kernel32.dll")] static extern IntPtr GlobalAlloc(uint flags,UIntPtr bytes);
  [DllImport("kernel32.dll")] static extern IntPtr GlobalLock(IntPtr handle);
  [DllImport("kernel32.dll")] static extern bool GlobalUnlock(IntPtr handle);
  static readonly short Descriptor=unchecked((short)RegisterClipboardFormat("FileGroupDescriptorW"));
  static readonly short Contents=unchecked((short)RegisterClipboardFormat("FileContents"));
  public readonly PromisedFile[] Files;
  public volatile bool Active;bool asyncMode=true;public int Result;
  public VirtualFiles(string[] names,long[] sizes){Files=new PromisedFile[names.Length];for(int i=0;i<names.Length;i++)Files[i]=new PromisedFile{Name=names[i],Size=sizes[i],ReadComplete=sizes[i]==0};}
  static FORMATETC Format(short id,TYMED medium,int index){return new FORMATETC{cfFormat=id,dwAspect=DVASPECT.DVASPECT_CONTENT,lindex=index,tymed=medium};}
  public int QueryGetData(ref FORMATETC format){
   if(format.dwAspect!=DVASPECT.DVASPECT_CONTENT)return unchecked((int)0x80040064);
   if(format.cfFormat==Descriptor&&(format.tymed&TYMED.TYMED_HGLOBAL)!=0)return 0;
   if(format.cfFormat==Contents&&(format.tymed&TYMED.TYMED_ISTREAM)!=0&&format.lindex>=0&&format.lindex<Files.Length)return 0;
   return unchecked((int)0x80040064);
  }
  public void GetData(ref FORMATETC format,out STGMEDIUM medium){
   int hr=QueryGetData(ref format);if(hr!=0)Marshal.ThrowExceptionForHR(hr);
   medium=new STGMEDIUM();
   if(format.cfFormat==Contents){medium.tymed=TYMED.TYMED_ISTREAM;medium.unionmember=Marshal.GetComInterfaceForObject(new PromiseStream(Files[format.lindex]),typeof(IStream));return;}
   byte[] bytes=new byte[4+592*Files.Length];
   using(var memory=new MemoryStream(bytes))using(var writer=new BinaryWriter(memory,System.Text.Encoding.Unicode)){
    writer.Write(Files.Length);
    for(int i=0;i<Files.Length;i++){
     long start=4+592*i;memory.Position=start;writer.Write(0x00004044); // FD_PROGRESSUI | FILESIZE | ATTRIBUTES
     memory.Position=start+36;writer.Write(0x80); // FILE_ATTRIBUTE_NORMAL
     memory.Position=start+64;writer.Write((uint)((ulong)Files[i].Size>>32));writer.Write((uint)Files[i].Size);
     memory.Position=start+72;writer.Write(System.Text.Encoding.Unicode.GetBytes(Files[i].Name));
    }
   }
   IntPtr handle=GlobalAlloc(0x42,new UIntPtr((uint)bytes.Length));if(handle==IntPtr.Zero)throw new OutOfMemoryException();
   IntPtr pointer=GlobalLock(handle);Marshal.Copy(bytes,0,pointer,bytes.Length);GlobalUnlock(handle);
   medium.tymed=TYMED.TYMED_HGLOBAL;medium.unionmember=handle;
  }
  public void GetDataHere(ref FORMATETC f,ref STGMEDIUM m){throw new NotSupportedException();}
  public int GetCanonicalFormatEtc(ref FORMATETC input,out FORMATETC output){output=input;output.ptd=IntPtr.Zero;return 0x40130;}
  public void SetData(ref FORMATETC f,ref STGMEDIUM m,bool release){if(release)ReleaseStgMedium(ref m);}
  [DllImport("ole32.dll")] static extern void ReleaseStgMedium(ref STGMEDIUM medium);
  public IEnumFORMATETC EnumFormatEtc(DATADIR direction){if(direction!=DATADIR.DATADIR_GET)throw new NotSupportedException();return new Formats(new[]{Format(Descriptor,TYMED.TYMED_HGLOBAL,-1),Format(Contents,TYMED.TYMED_ISTREAM,0)});}
  public int DAdvise(ref FORMATETC f,ADVF flags,IAdviseSink sink,out int connection){connection=0;return unchecked((int)0x80040003);}
  public void DUnadvise(int connection){}
  public int EnumDAdvise(out IEnumSTATDATA values){values=null;return unchecked((int)0x80040003);}
  public int SetAsyncMode(bool enabled){asyncMode=enabled;return 0;}
  public int GetAsyncMode(out bool enabled){enabled=asyncMode;return 0;}
  public int StartOperation(IBindCtx context){Active=true;return 0;}
  public int InOperation(out bool active){active=Active;return 0;}
  public int EndOperation(int result,IBindCtx context,uint effect){Result=result;Active=false;return 0;}
  public void Cancel(){foreach(var file in Files){file.Error=new IOException("Transfer cancelled");file.Ready.Set();}}
 }
 [ComVisible(true),ClassInterface(ClassInterfaceType.None)]
 public class NativeDrag : IDropSource {
  static int state;
  [DllImport("user32.dll")] static extern void mouse_event(uint flags,uint x,uint y,uint data,UIntPtr extra);
  [DllImport("ole32.dll")] static extern int OleInitialize(IntPtr reserved);
  [DllImport("ole32.dll")] static extern void OleUninitialize();
  [DllImport("ole32.dll")] static extern int DoDragDrop([MarshalAs(UnmanagedType.Interface)] System.Runtime.InteropServices.ComTypes.IDataObject data,[MarshalAs(UnmanagedType.Interface)] IDropSource source,uint allowed,out uint effect);
  public int QueryContinueDrag(bool escape,uint keys){if(escape||Volatile.Read(ref state)==2)return 0x40101;return Volatile.Read(ref state)==1?0x40100:0;}
  public int GiveFeedback(uint effect){return 0x40102;}
  public static void Run(string[] names,long[] sizes){
   int hr=OleInitialize(IntPtr.Zero);if(hr<0)Marshal.ThrowExceptionForHR(hr);
   VirtualFiles data=null;
   try{
    if(names==null||sizes==null||names.Length==0||names.Length>100||names.Length!=sizes.Length)throw new Exception("Invalid file promises");
    for(int i=0;i<names.Length;i++)if(names[i].Length>180||Path.GetFileName(names[i])!=names[i]||sizes[i]<0||sizes[i]>1073741824)throw new Exception("Invalid promised file");
    data=new VirtualFiles(names,sizes);state=0;
    var reader=new Thread(delegate(){
     try{string line;var json=new JavaScriptSerializer();
      while((line=Console.ReadLine())!=null){
       if(line=="drop"){Interlocked.Exchange(ref state,1);mouse_event(4,0,0,0,new UIntPtr(0x584B564D));}
       else if(line=="cancel"){data.Cancel();Interlocked.Exchange(ref state,2);mouse_event(4,0,0,0,new UIntPtr(0x584B564D));return;}
       else {var value=json.Deserialize<System.Collections.Generic.Dictionary<string,object>>(line);int index=Convert.ToInt32(value["index"]);string file=(string)value["path"];
        if(index<0||index>=data.Files.Length||!Path.IsPathRooted(file)||new FileInfo(file).Length!=data.Files[index].Size)throw new IOException("Invalid delivered file");
        data.Files[index].Path=file;data.Files[index].Ready.Set();}
      }
     }catch(Exception){ }data.Cancel();Interlocked.Exchange(ref state,2);mouse_event(4,0,0,0,new UIntPtr(0x584B564D));
    });reader.IsBackground=true;reader.Start();
    Console.WriteLine("ready");Console.Out.Flush();uint effect;
    int result=DoDragDrop(data,new NativeDrag(),1,out effect);
    DateTime deadline=DateTime.UtcNow.AddMinutes(30);
    while(data.Active&&DateTime.UtcNow<deadline&&Volatile.Read(ref state)!=2){Application.DoEvents();Thread.Sleep(10);}
    bool complete=result==0x40100&&effect==1&&!data.Active&&data.Result>=0;
    foreach(var file in data.Files)complete=complete&&file.ReadComplete;
    Console.WriteLine(complete?"dropped":"cancelled");Console.Out.Flush();
   }finally{if(data!=null)data.Cancel();OleUninitialize();}
  }
 }
}
