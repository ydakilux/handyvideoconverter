package dashboard

func renderHTML(jsonData string) string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Video Converter Dashboard</title>
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{
--bg-primary:#0f1119;
--bg-secondary:#161825;
--bg-card:#1c1f2e;
--bg-card-hover:#222640;
--bg-table-row:#1a1d2c;
--bg-table-row-alt:#161825;
--text-primary:#e2e8f0;
--text-secondary:#94a3b8;
--text-muted:#64748b;
--border-color:#2a2d3e;
--accent-green:#4ade80;
--accent-green-dim:rgba(74,222,128,0.12);
--accent-red:#ef4444;
--accent-red-dim:rgba(239,68,68,0.12);
--accent-orange:#f59e0b;
--accent-orange-dim:rgba(245,158,11,0.12);
--accent-blue:#3b82f6;
--accent-blue-dim:rgba(59,130,246,0.12);
--accent-purple:#a855f7;
--accent-purple-dim:rgba(168,85,247,0.12);
--accent-cyan:#22d3ee;
--radius:10px;
--radius-sm:6px;
--shadow:0 2px 8px rgba(0,0,0,0.3);
--font-sans:"Segoe UI","SF Pro Display","Helvetica Neue",system-ui,sans-serif;
--font-mono:"Cascadia Code","SF Mono","Fira Code",monospace;
--transition:0.25s cubic-bezier(0.4,0,0.2,1);
}
html{font-size:14px;scroll-behavior:smooth}
body{font-family:var(--font-sans);background:var(--bg-primary);color:var(--text-primary);line-height:1.6;min-height:100vh}
a{color:var(--accent-cyan);text-decoration:none}
.dashboard{max-width:1600px;margin:0 auto;padding:0 24px 48px}
.header{display:flex;align-items:center;justify-content:space-between;padding:20px 0;border-bottom:1px solid var(--border-color);margin-bottom:24px;flex-wrap:wrap;gap:12px}
.header-left h1{font-size:1.5rem;font-weight:700;letter-spacing:-0.02em;background:linear-gradient(135deg,var(--accent-cyan),var(--accent-purple));-webkit-background-clip:text;-webkit-text-fill-color:transparent;background-clip:text}
.header-left .timestamp{font-size:0.8rem;color:var(--text-muted);margin-top:2px}
.header-right{display:flex;align-items:center;gap:12px}
.drive-select{background:var(--bg-card);color:var(--text-primary);border:1px solid var(--border-color);padding:8px 14px;border-radius:var(--radius-sm);font-family:var(--font-sans);font-size:0.85rem;cursor:pointer;outline:none;transition:border-color var(--transition)}
.drive-select:hover,.drive-select:focus{border-color:var(--accent-cyan)}
.kpi-row{display:grid;grid-template-columns:repeat(6,1fr);gap:14px;margin-bottom:24px}
@media(max-width:1200px){.kpi-row{grid-template-columns:repeat(3,1fr)}}
@media(max-width:700px){.kpi-row{grid-template-columns:repeat(2,1fr)}}
.kpi-card{background:var(--bg-card);border:1px solid var(--border-color);border-radius:var(--radius);padding:20px;position:relative;overflow:hidden;transition:transform var(--transition),box-shadow var(--transition)}
.kpi-card:hover{transform:translateY(-2px);box-shadow:var(--shadow)}
.kpi-card::before{content:"";position:absolute;top:0;left:0;right:0;height:3px}
.kpi-card.green::before{background:var(--accent-green)}
.kpi-card.red::before{background:var(--accent-red)}
.kpi-card.orange::before{background:var(--accent-orange)}
.kpi-card.blue::before{background:var(--accent-blue)}
.kpi-card.purple::before{background:var(--accent-purple)}
.kpi-card.cyan::before{background:var(--accent-cyan)}
.kpi-value{font-size:2rem;font-weight:800;letter-spacing:-0.03em;line-height:1.1}
.kpi-card.green .kpi-value{color:var(--accent-green)}
.kpi-card.red .kpi-value{color:var(--accent-red)}
.kpi-card.orange .kpi-value{color:var(--accent-orange)}
.kpi-card.blue .kpi-value{color:var(--accent-blue)}
.kpi-card.purple .kpi-value{color:var(--accent-purple)}
.kpi-card.cyan .kpi-value{color:var(--accent-cyan)}
.kpi-label{font-size:0.78rem;color:var(--text-muted);margin-top:6px;text-transform:uppercase;letter-spacing:0.06em;font-weight:600}
.two-col{display:grid;grid-template-columns:1fr 1fr;gap:20px;margin-bottom:24px}
@media(max-width:900px){.two-col{grid-template-columns:1fr}}
.panel{background:var(--bg-card);border:1px solid var(--border-color);border-radius:var(--radius);padding:24px;transition:box-shadow var(--transition)}
.panel:hover{box-shadow:var(--shadow)}
.panel-title{font-size:1rem;font-weight:700;margin-bottom:16px;color:var(--text-primary);letter-spacing:-0.01em}
.savings-stack{display:flex;flex-direction:column;gap:12px}
.savings-box{background:var(--bg-secondary);border:1px solid var(--border-color);border-radius:var(--radius-sm);padding:16px 20px;display:flex;justify-content:space-between;align-items:center}
.savings-box .label{font-size:0.82rem;color:var(--text-muted);text-transform:uppercase;letter-spacing:0.05em;font-weight:600}
.savings-box .value{font-size:1.35rem;font-weight:800;color:var(--accent-cyan)}
.chart-container{width:100%;height:320px}
.full-chart{margin-bottom:24px}
.full-chart .chart-container{height:360px}
.section{margin-bottom:24px}
.section-header{background:var(--bg-card);border:1px solid var(--border-color);border-radius:var(--radius) var(--radius) 0 0;padding:14px 20px;cursor:pointer;display:flex;align-items:center;justify-content:space-between;user-select:none;transition:background var(--transition)}
.section-header:hover{background:var(--bg-card-hover)}
.section-header h2{font-size:0.95rem;font-weight:700;display:flex;align-items:center;gap:8px}
.section-header .toggle{font-size:0.75rem;color:var(--text-muted);transition:transform var(--transition)}
.section-header.collapsed .toggle{transform:rotate(-90deg)}
.section-header.error-tint{border-left:3px solid var(--accent-red);background:linear-gradient(90deg,rgba(239,68,68,0.06),transparent)}
.section-header.warning-tint{border-left:3px solid var(--accent-orange);background:linear-gradient(90deg,rgba(245,158,11,0.06),transparent)}
.section-body{background:var(--bg-secondary);border:1px solid var(--border-color);border-top:none;border-radius:0 0 var(--radius) var(--radius);overflow:hidden;transition:max-height 0.35s ease,opacity 0.25s ease}
.section-body.hidden{max-height:0!important;opacity:0;overflow:hidden;border:none}
table{width:100%;border-collapse:collapse;font-size:0.82rem}
thead{position:sticky;top:0;z-index:2}
thead th{background:var(--bg-card);color:var(--text-muted);font-weight:700;text-transform:uppercase;letter-spacing:0.05em;font-size:0.72rem;padding:10px 14px;text-align:left;border-bottom:2px solid var(--border-color);cursor:pointer;white-space:nowrap;user-select:none}
thead th:hover{color:var(--text-primary)}
thead th .sort-arrow{margin-left:4px;font-size:0.65rem;opacity:0.5}
thead th.sorted .sort-arrow{opacity:1;color:var(--accent-cyan)}
tbody tr{transition:background var(--transition)}
tbody tr:nth-child(odd){background:var(--bg-table-row)}
tbody tr:nth-child(even){background:var(--bg-table-row-alt)}
tbody tr:hover{background:var(--bg-card-hover)}
td{padding:8px 14px;border-bottom:1px solid var(--border-color);color:var(--text-secondary);white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:350px}
td.path-cell{font-family:var(--font-mono);font-size:0.78rem}
.status-badge{display:inline-block;padding:2px 10px;border-radius:20px;font-size:0.72rem;font-weight:700;letter-spacing:0.03em}
.status-success{background:var(--accent-green-dim);color:var(--accent-green)}
.status-error{background:var(--accent-red-dim);color:var(--accent-red)}
.status-warning{background:var(--accent-orange-dim);color:var(--accent-orange)}
.status-info{background:var(--accent-blue-dim);color:var(--accent-blue)}
.btn{background:var(--bg-card);color:var(--text-secondary);border:1px solid var(--border-color);padding:8px 18px;border-radius:var(--radius-sm);cursor:pointer;font-family:var(--font-sans);font-size:0.8rem;transition:all var(--transition)}
.btn:hover{background:var(--bg-card-hover);color:var(--text-primary);border-color:var(--accent-cyan)}
.show-all-row{padding:12px 20px;text-align:center;background:var(--bg-secondary)}
.no-data{padding:32px;text-align:center;color:var(--text-muted);font-size:0.9rem}
</style>
</head>
<body>
<div class="dashboard" id="app">
<div class="header">
<div class="header-left">
<h1>Video Converter Dashboard</h1>
<div class="timestamp" id="timestamp"></div>
</div>
<div class="header-right">
<select class="drive-select" id="driveFilter" onchange="onDriveChange()">
<option value="">All Drives</option>
</select>
</div>
</div>
<div class="kpi-row" id="kpiRow"></div>
<div class="two-col">
<div class="panel">
<div class="panel-title">Space Savings</div>
<div class="chart-container" id="donutChart"></div>
</div>
<div class="panel">
<div class="panel-title">Savings Summary</div>
<div class="savings-stack" id="savingsStack"></div>
</div>
</div>
<div class="panel full-chart">
<div class="panel-title">Space Saved Over Time</div>
<div class="chart-container" id="timelineChart"></div>
</div>
<div class="panel full-chart">
<div class="panel-title">Format Breakdown</div>
<div class="chart-container" id="formatChart"></div>
</div>
<div class="section" id="recentSection">
<div class="section-header" onclick="toggleSection('recentBody')">
<h2>Recent Conversions <span id="recentCount" style="color:var(--text-muted);font-weight:400"></span></h2>
<span class="toggle">&#9660;</span>
</div>
<div class="section-body" id="recentBody"></div>
</div>
<div class="section" id="errorSection" style="display:none">
<div class="section-header error-tint" onclick="toggleSection('errorBody')">
<h2>&#9888; Errors <span id="errorCount" style="color:var(--text-muted);font-weight:400"></span></h2>
<span class="toggle">&#9660;</span>
</div>
<div class="section-body" id="errorBody"></div>
</div>
<div class="section" id="nbSection" style="display:none">
<div class="section-header warning-tint" onclick="toggleSection('nbBody')">
<h2>&#9888; Not Beneficial <span id="nbCount" style="color:var(--text-muted);font-weight:400"></span></h2>
<span class="toggle">&#9660;</span>
</div>
<div class="section-body" id="nbBody"></div>
</div>
</div>
<script src="https://cdn.jsdelivr.net/npm/echarts@5/dist/echarts.min.js"></script>
<script>
const DATA = ` + jsonData + `;

function formatBytes(b){
if(b==null||isNaN(b))return"0 B";
if(b===0)return"0 B";
var k=1024,s=["B","KB","MB","GB","TB","PB"],i=Math.floor(Math.log(Math.abs(b))/Math.log(k));
if(i<0)i=0;if(i>=s.length)i=s.length-1;
return(b/Math.pow(k,i)).toFixed(i>0?2:0)+" "+s[i];
}

function truncatePath(p,n){
if(!p)return"";
n=n||60;
if(p.length<=n)return p;
return"..."+p.slice(-n);
}

function pct(a,b){return b>0?((a/b)*100).toFixed(1):0;}

var selectedDrive="";
var sortStates={};
var showAllRecent=false;

function onDriveChange(){
selectedDrive=document.getElementById("driveFilter").value;
showAllRecent=false;
renderAll();
}

function getFilteredRecent(){
if(!selectedDrive)return DATA.recent||[];
return(DATA.recent||[]).filter(function(r){return r.DriveRoot===selectedDrive;});
}
function getFilteredErrors(){
if(!selectedDrive)return DATA.errors||[];
return(DATA.errors||[]).filter(function(r){return r.DriveRoot===selectedDrive;});
}
function getFilteredNB(){
if(!selectedDrive)return DATA.notBeneficial||[];
return(DATA.notBeneficial||[]).filter(function(r){return r.DriveRoot===selectedDrive;});
}
function getFilteredTimeline(){
if(!selectedDrive)return DATA.timeline||[];
return(DATA.timeline||[]).filter(function(r){return r.DriveRoot===selectedDrive;});
}
function getFilteredFormats(){
if(!selectedDrive)return DATA.formats||[];
return(DATA.formats||[]).filter(function(r){return true;});
}

function computeKPI(){
if(!selectedDrive){
var s=DATA.stats||{};
var totalOrig=s.TotalOriginal||0;
var totalConv=s.TotalConverted||0;
var savedPct=totalOrig>0?pct(totalOrig-totalConv,totalOrig):"0.0";
return{total:s.TotalFiles||0,success:s.SuccessCount||0,errors:s.ErrorCount||0,nb:s.NotBeneficial||0,hevc:s.AlreadyHEVC||0,savedPct:savedPct};
}
var recs=getFilteredRecent();
var errs=getFilteredErrors();
var nb=getFilteredNB();
var successCount=0,errCount=0,nbCount=0,hevcCount=0,origSum=0,convSum=0;
recs.forEach(function(r){
if(r.Error){errCount++;}
else if(r.Note==="not_beneficial"){nbCount++;}
else if(r.Note==="already_hevc"){hevcCount++;}
else{successCount++;origSum+=(r.OriginalSize||0);convSum+=(r.ConvertedSize||0);}
});
errCount+=errs.length;
nbCount+=nb.length;
var total=recs.length;
var savedPct=origSum>0?pct(origSum-convSum,origSum):"0.0";
return{total:total,success:successCount,errors:errCount,nb:nbCount,hevc:hevcCount,savedPct:savedPct};
}

function renderKPI(){
var k=computeKPI();
var cards=[
{value:k.total,label:"Total Files",cls:"cyan"},
{value:k.success,label:"Successful",cls:"green"},
{value:k.errors,label:"Errors",cls:"red"},
{value:k.nb,label:"Not Beneficial",cls:"orange"},
{value:k.hevc,label:"Already HEVC",cls:"blue"},
{value:k.savedPct+"%",label:"Space Saved",cls:"purple"}
];
var html="";
cards.forEach(function(c){
html+='<div class="kpi-card '+c.cls+'"><div class="kpi-value">'+c.value+'</div><div class="kpi-label">'+c.label+'</div></div>';
});
document.getElementById("kpiRow").innerHTML=html;
}

function renderSavingsStack(){
var w=DATA.spaceSavedWeek||{};
var m=DATA.spaceSavedMonth||{};
var t=DATA.spaceSaved||{};
var items=[
{label:"This Week",value:formatBytes(w.BytesSaved||0),count:w.FileCount||0},
{label:"This Month",value:formatBytes(m.BytesSaved||0),count:m.FileCount||0},
{label:"All Time",value:formatBytes(t.BytesSaved||0),count:t.FileCount||0}
];
var html="";
items.forEach(function(it){
html+='<div class="savings-box"><div><div class="label">'+it.label+'</div><div style="font-size:0.75rem;color:var(--text-muted);margin-top:2px">'+it.count+' files</div></div><div class="value">'+it.value+'</div></div>';
});
document.getElementById("savingsStack").innerHTML=html;
}

var donutInstance,timelineInstance,formatInstance;

function renderDonut(){
var s=DATA.stats||{};
var orig=s.TotalOriginal||0;
var conv=s.TotalConverted||0;
var saved=orig-conv;
if(selectedDrive){
var recs=getFilteredRecent().filter(function(r){return!r.Error&&r.Note!==""?false:true;});
orig=0;conv=0;
getFilteredRecent().forEach(function(r){
if(!r.Error){orig+=(r.OriginalSize||0);conv+=(r.ConvertedSize||0);}
});
saved=orig-conv;
}
var el=document.getElementById("donutChart");
if(!donutInstance)donutInstance=echarts.init(el,null,{renderer:"canvas"});
donutInstance.setOption({
tooltip:{trigger:"item",formatter:function(p){return p.name+": "+formatBytes(p.value)+" ("+p.percent+"%)";},backgroundColor:"#1c1f2e",borderColor:"#2a2d3e",textStyle:{color:"#e2e8f0"}},
series:[{
type:"pie",radius:["55%","80%"],center:["50%","50%"],
itemStyle:{borderRadius:6,borderColor:"#0f1119",borderWidth:3},
label:{show:true,position:"center",formatter:function(){return saved>0?formatBytes(saved)+"\nSaved":"No Savings";},fontSize:16,fontWeight:"bold",color:"#e2e8f0",lineHeight:22},
emphasis:{label:{fontSize:18}},
data:[
{value:conv,name:"Remaining",itemStyle:{color:"#334155"}},
{value:saved>0?saved:0,name:"Saved",itemStyle:{color:new echarts.graphic.LinearGradient(0,0,1,1,[{offset:0,color:"#22d3ee"},{offset:1,color:"#a855f7"}])}}
]
}]
});
}

function renderTimeline(){
var tl=DATA.timeline||[];
var filtered=getFilteredTimeline();
var drives=selectedDrive?[selectedDrive]:(DATA.driveRoots||[]);
var dateSet={};
filtered.forEach(function(p){dateSet[p.Date]=true;});
var dates=Object.keys(dateSet).sort();
var series=[];
var colors=["#22d3ee","#a855f7","#4ade80","#f59e0b","#ef4444","#3b82f6"];
drives.forEach(function(d,i){
var dataMap={};
filtered.filter(function(p){return p.DriveRoot===d;}).forEach(function(p){dataMap[p.Date]=(dataMap[p.Date]||0)+(p.BytesSaved||0);});
var vals=dates.map(function(dt){return dataMap[dt]||0;});
series.push({
name:d,type:"bar",stack:"total",
data:vals,
itemStyle:{color:colors[i%colors.length],borderRadius:[2,2,0,0]},
emphasis:{focus:"series"}
});
});
var el=document.getElementById("timelineChart");
if(!timelineInstance)timelineInstance=echarts.init(el,null,{renderer:"canvas"});
timelineInstance.setOption({
tooltip:{trigger:"axis",backgroundColor:"#1c1f2e",borderColor:"#2a2d3e",textStyle:{color:"#e2e8f0"},
formatter:function(params){var s=params[0].axisValue+"<br/>";params.forEach(function(p){if(p.value>0)s+=p.marker+" "+p.seriesName+": "+formatBytes(p.value)+"<br/>";});return s;}},
legend:{show:drives.length>1,textStyle:{color:"#94a3b8"},top:0},
grid:{left:80,right:20,top:drives.length>1?40:20,bottom:40},
xAxis:{type:"category",data:dates,axisLine:{lineStyle:{color:"#2a2d3e"}},axisLabel:{color:"#64748b",fontSize:11}},
yAxis:{type:"value",axisLine:{show:false},splitLine:{lineStyle:{color:"#1c1f2e"}},axisLabel:{color:"#64748b",fontSize:11,formatter:function(v){return formatBytes(v);}}},
series:series
},true);
}

function renderFormats(){
var fmts=getFilteredFormats();
var codecs=[];var counts=[];
fmts.sort(function(a,b){return b.Count-a.Count;});
fmts.forEach(function(f){codecs.push(f.SourceCodec+(f.SourceContainer?" ("+f.SourceContainer+")":""));counts.push(f.Count);});
var el=document.getElementById("formatChart");
if(!formatInstance)formatInstance=echarts.init(el,null,{renderer:"canvas"});
formatInstance.setOption({
tooltip:{trigger:"axis",backgroundColor:"#1c1f2e",borderColor:"#2a2d3e",textStyle:{color:"#e2e8f0"}},
grid:{left:140,right:40,top:10,bottom:20},
xAxis:{type:"value",axisLine:{lineStyle:{color:"#2a2d3e"}},splitLine:{lineStyle:{color:"#1c1f2e"}},axisLabel:{color:"#64748b"}},
yAxis:{type:"category",data:codecs,axisLine:{lineStyle:{color:"#2a2d3e"}},axisLabel:{color:"#e2e8f0",fontSize:12}},
series:[{type:"bar",data:counts,
itemStyle:{color:new echarts.graphic.LinearGradient(0,0,1,0,[{offset:0,color:"#3b82f6"},{offset:1,color:"#22d3ee"}]),borderRadius:[0,4,4,0]},
barMaxWidth:28,
label:{show:true,position:"right",color:"#94a3b8",fontSize:11}
}]
},true);
}

function getStatus(r){
if(r.Error)return{text:"Error",cls:"status-error"};
if(r.Note==="not_beneficial")return{text:"Not Beneficial",cls:"status-warning"};
if(r.Note==="already_hevc")return{text:"Already HEVC",cls:"status-info"};
return{text:"Success",cls:"status-success"};
}

function sortData(arr,key,tableId){
if(!sortStates[tableId])sortStates[tableId]={key:key,asc:true};
else if(sortStates[tableId].key===key)sortStates[tableId].asc=!sortStates[tableId].asc;
else sortStates[tableId]={key:key,asc:true};
var asc=sortStates[tableId].asc;
arr.sort(function(a,b){
var va=typeof key==="function"?key(a):a[key];
var vb=typeof key==="function"?key(b):b[key];
if(typeof va==="string")va=va.toLowerCase();
if(typeof vb==="string")vb=vb.toLowerCase();
if(va<vb)return asc?-1:1;
if(va>vb)return asc?1:-1;
return 0;
});
}

function arrow(tableId,key){
if(!sortStates[tableId]||sortStates[tableId].key!==key)return'<span class="sort-arrow">&#9650;&#9660;</span>';
return'<span class="sort-arrow">'+(sortStates[tableId].asc?"&#9650;":"&#9660;")+'</span>';
}

function renderRecent(){
var recs=getFilteredRecent();
document.getElementById("recentCount").textContent="("+recs.length+")";
if(!recs.length){document.getElementById("recentBody").innerHTML='<div class="no-data">No recent conversions</div>';return;}
var limit=showAllRecent?recs.length:Math.min(50,recs.length);
var html='<table><thead><tr>';
html+='<th onclick="sortRecentBy(\'SourcePath\')">Source Path '+arrow("recent","SourcePath")+'</th>';
html+='<th onclick="sortRecentBy(\'OriginalSize\')">Original '+arrow("recent","OriginalSize")+'</th>';
html+='<th onclick="sortRecentBy(\'ConvertedSize\')">Converted '+arrow("recent","ConvertedSize")+'</th>';
html+='<th onclick="sortRecentBy(\'_savings\')">Savings % '+arrow("recent","_savings")+'</th>';
html+='<th onclick="sortRecentBy(\'SourceCodec\')">Codec '+arrow("recent","SourceCodec")+'</th>';
html+='<th onclick="sortRecentBy(\'_status\')">Status '+arrow("recent","_status")+'</th>';
html+='<th onclick="sortRecentBy(\'ConvertedAt\')">Date '+arrow("recent","ConvertedAt")+'</th>';
html+='</tr></thead><tbody>';
for(var i=0;i<limit;i++){
var r=recs[i];
var st=getStatus(r);
var savings=r.OriginalSize>0?((1-(r.ConvertedSize||0)/r.OriginalSize)*100).toFixed(1):"0.0";
html+='<tr>';
html+='<td class="path-cell" title="'+(r.SourcePath||"").replace(/"/g,"&quot;")+'">'+truncatePath(r.SourcePath,60)+'</td>';
html+='<td>'+formatBytes(r.OriginalSize)+'</td>';
html+='<td>'+formatBytes(r.ConvertedSize)+'</td>';
html+='<td>'+(r.Error?"—":savings+"%")+'</td>';
html+='<td>'+(r.SourceCodec||"—")+'</td>';
html+='<td><span class="status-badge '+st.cls+'">'+st.text+'</span></td>';
html+='<td>'+(r.ConvertedAt||"—")+'</td>';
html+='</tr>';
}
html+='</tbody></table>';
if(!showAllRecent&&recs.length>50){
html+='<div class="show-all-row"><button class="btn" onclick="showAllRecent=true;renderRecent();">Show All ('+recs.length+')</button></div>';
}
document.getElementById("recentBody").innerHTML=html;
}

function sortRecentBy(key){
var recs=getFilteredRecent();
if(key==="_savings"){
sortData(recs,function(r){return r.OriginalSize>0?(1-(r.ConvertedSize||0)/r.OriginalSize):0;},"recent");
}else if(key==="_status"){
sortData(recs,function(r){return getStatus(r).text;},"recent");
}else{
sortData(recs,key,"recent");
}
renderRecent();
}

function renderErrors(){
var errs=getFilteredErrors();
var sec=document.getElementById("errorSection");
if(!errs.length){sec.style.display="none";return;}
sec.style.display="";
document.getElementById("errorCount").textContent="("+errs.length+")";
var html='<table><thead><tr><th>Source Path</th><th>Size</th><th>Error</th><th>Date</th></tr></thead><tbody>';
errs.forEach(function(e){
html+='<tr>';
html+='<td class="path-cell" title="'+(e.SourcePath||"").replace(/"/g,"&quot;")+'">'+truncatePath(e.SourcePath,60)+'</td>';
html+='<td>'+formatBytes(e.OriginalSize)+'</td>';
html+='<td style="white-space:normal;max-width:400px;color:var(--accent-red)">'+(e.Error||"Unknown error")+'</td>';
html+='<td>'+(e.ConvertedAt||"—")+'</td>';
html+='</tr>';
});
html+='</tbody></table>';
document.getElementById("errorBody").innerHTML=html;
}

function renderNB(){
var nb=getFilteredNB();
var sec=document.getElementById("nbSection");
if(!nb.length){sec.style.display="none";return;}
sec.style.display="";
document.getElementById("nbCount").textContent="("+nb.length+")";
var html='<table><thead><tr><th>Source Path</th><th>Original</th><th>Converted</th><th>Increase %</th></tr></thead><tbody>';
nb.forEach(function(r){
var inc=r.OriginalSize>0?(((r.ConvertedSize-r.OriginalSize)/r.OriginalSize)*100).toFixed(1):"0.0";
html+='<tr>';
html+='<td class="path-cell" title="'+(r.SourcePath||"").replace(/"/g,"&quot;")+'">'+truncatePath(r.SourcePath,60)+'</td>';
html+='<td>'+formatBytes(r.OriginalSize)+'</td>';
html+='<td>'+formatBytes(r.ConvertedSize)+'</td>';
html+='<td style="color:var(--accent-orange)">+'+inc+'%</td>';
html+='</tr>';
});
html+='</tbody></table>';
document.getElementById("nbBody").innerHTML=html;
}

function toggleSection(id){
var body=document.getElementById(id);
var header=body.previousElementSibling;
if(body.classList.contains("hidden")){
body.classList.remove("hidden");
body.style.maxHeight=body.scrollHeight+"px";
header.classList.remove("collapsed");
}else{
body.classList.add("hidden");
header.classList.add("collapsed");
}
}

function renderAll(){
renderKPI();
renderDonut();
renderSavingsStack();
renderTimeline();
renderFormats();
renderRecent();
renderErrors();
renderNB();
}

function init(){
document.getElementById("timestamp").textContent="Generated: "+(DATA.generatedAt||"Unknown");
var sel=document.getElementById("driveFilter");
(DATA.driveRoots||[]).forEach(function(d){
var opt=document.createElement("option");
opt.value=d;opt.textContent=d;
sel.appendChild(opt);
});
renderAll();
window.addEventListener("resize",function(){
if(donutInstance)donutInstance.resize();
if(timelineInstance)timelineInstance.resize();
if(formatInstance)formatInstance.resize();
});
}

init();
</script>
</body>
</html>`
}
