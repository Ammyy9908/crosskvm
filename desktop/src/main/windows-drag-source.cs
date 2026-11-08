using System;
using System.Text;
using System.Threading;
using System.Drawing;
using System.Windows.Forms;
using System.Runtime.InteropServices;
using System.Web.Script.Serialization;
namespace CrossKVM {
 public class EdgeCapture : Form {
  [DllImport("user32.dll")] static extern IntPtr GetForegroundWindow();
  [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern int GetClassName(IntPtr window,StringBuilder name,int count);
  [DllImport("user32.dll")] static extern void keybd_event(byte key,byte scan,uint flags,UIntPtr extra);
  static readonly JavaScriptSerializer json=new JavaScriptSerializer();
  static string side="none";
  static EdgeCapture strip;
  static bool held, captured, cancelled;
  static int change;
  static int gesture;
  static string[] files=new string[0];
  static Point origin;
  static void Emit() { Console.WriteLine(json.Serialize(new {change=change,gesture=gesture,down=held,files=files}));Console.Out.Flush(); }
  protected override bool ShowWithoutActivation { get {return true;} }
  protected override CreateParams CreateParams { get {var p=base.CreateParams;p.ExStyle|=0x08000000|0x80;return p;} }
  EdgeCapture() {
   FormBorderStyle=FormBorderStyle.None;ShowInTaskbar=false;TopMost=true;
   Opacity=0.02;BackColor=Color.Black;AllowDrop=true;
   DragEnter+=(s,e)=> {
    // Obtain actual OLE drag data, never Explorer selection or the clipboard.
    e.Effect=DragDropEffects.None;
    if(!captured && held && !cancelled && e.Data.GetDataPresent(DataFormats.FileDrop)) {
     var paths=e.Data.GetData(DataFormats.FileDrop) as string[];
     if(paths!=null && paths.Length>0 && paths.Length<=100) {
      files=paths;captured=true;change++;Emit();
     }
    }
   };
   DragOver+=(s,e)=>{e.Effect=DragDropEffects.None;};
   DragDrop+=(s,e)=>{e.Effect=DragDropEffects.None;};
  }
  static void CancelDrag() {
   if(captured && !cancelled) {
    cancelled=true;
    keybd_event(0x1B,0,0,new UIntPtr(0x584B564D));
    keybd_event(0x1B,0,2,new UIntPtr(0x584B564D));
   }
   strip.Hide();
  }
  public static void Run() {
   Application.EnableVisualStyles();strip=new EdgeCapture();
   // Create the handle without displaying the edge surface.
   IntPtr handle=strip.Handle;
   var reader=new Thread(delegate() {
    string line;
    while((line=Console.ReadLine())!=null) {
     string command=line;
     strip.BeginInvoke((Action)(()=> {
      if(command.StartsWith("down:")) {
       gesture=int.Parse(command.Substring(5));held=true;origin=Cursor.Position;captured=false;cancelled=false;files=new string[0];Emit();
      }
      else if(command=="up") {held=false;strip.Hide();files=new string[0];captured=false;cancelled=false;Emit();}
      else if(command=="cancel")CancelDrag();
      else if(command=="close")Application.ExitThread();
      else if(command=="left"||command=="right"||command=="top"||command=="bottom"||command=="none") {side=command;if(side=="none")strip.Hide();}
     }));
    }
    strip.BeginInvoke((Action)(()=>Application.ExitThread()));
   });reader.IsBackground=true;reader.Start();
   var timer=new System.Windows.Forms.Timer();timer.Interval=20;
   timer.Tick+=(s,e)=> {
    if(!held||cancelled||side=="none"){strip.Hide();return;}
    var name=new StringBuilder(128);GetClassName(GetForegroundWindow(),name,128);
    string cls=name.ToString();
    if(!captured && cls!="CabinetWClass" && cls!="ExploreWClass" && cls!="Progman" && cls!="WorkerW"){strip.Hide();return;}
    Point p=Cursor.Position;
    if(!captured && Math.Abs(p.X-origin.X)<SystemInformation.DragSize.Width && Math.Abs(p.Y-origin.Y)<SystemInformation.DragSize.Height)return;
    Rectangle b=Screen.PrimaryScreen.Bounds;
    const int width=32;
    if(side=="left")strip.Bounds=new Rectangle(b.Left,b.Top,width,b.Height);
    if(side=="right")strip.Bounds=new Rectangle(b.Right-width,b.Top,width,b.Height);
    if(side=="top")strip.Bounds=new Rectangle(b.Left,b.Top,b.Width,width);
    if(side=="bottom")strip.Bounds=new Rectangle(b.Left,b.Bottom-width,b.Width,width);
    if(!strip.Visible)strip.Show();
   };
   timer.Start();Emit();Application.Run();timer.Dispose();strip.Dispose();
  }
 }
}
