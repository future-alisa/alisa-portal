'use client';
import { useEffect, useState } from 'react';
import { ExternalLink, Layers3, Search, ShieldCheck, RefreshCw } from 'lucide-react';
type PublicService = { id: string; name: string; category: string; url: string; description: string; accessRequirement: 'none' | 'account' };
export function PublicCatalog({ connect }: { connect: (token: string) => void }) {
 const [services,setServices]=useState<PublicService[]>([]),[error,setError]=useState(''),[loading,setLoading]=useState(true),[search,setSearch]=useState(''),[filter,setFilter]=useState('all'),[login,setLogin]=useState(false),[token,setToken]=useState(''),[revision,setRevision]=useState(0);
 useEffect(()=>{ const controller=new AbortController(); setLoading(true); setError('');
  void fetch('/api/public/services',{signal:controller.signal,cache:'no-store'}).then(async response=>{if(!response.ok)throw new Error('公开目录暂时无法加载'); return response.json();}).then(data=>setServices(data.services)).catch(e=>{if(!controller.signal.aborted)setError(e.message);}).finally(()=>{if(!controller.signal.aborted)setLoading(false);});
  return ()=>controller.abort();
 },[revision]);
 const visible=services.filter(s=>(filter==='all'||s.accessRequirement===filter)&&`${s.name} ${s.category} ${s.description}`.toLowerCase().includes(search.toLowerCase()));
 return <div className="public-catalog"><header><a className="brand" href="/"><Layers3/> alisa.</a><button className="button" onClick={()=>setLogin(!login)}><ShieldCheck size={16}/>管理员连接</button></header><main>
 <div className="page-heading"><div><div className="eyebrow">ALISA · PUBLIC APPS</div><h1>发现好用的应用与服务</h1><p>无需登录门户即可浏览。需要账号的应用，请使用该服务自己的账号访问。</p></div><button className="button" disabled={loading} onClick={()=>setRevision(v=>v+1)}><RefreshCw size={16}/>刷新</button></div>
 {login&&<form className="panel public-login" onSubmit={e=>{e.preventDefault();if(token.trim())connect(token.trim());}}><label>管理员令牌<input type="password" autoComplete="off" required value={token} onChange={e=>setToken(e.target.value)}/></label><p className="form-note">仅管理资源需要令牌，令牌保存在当前页面内存中。</p><button className="button primary">进入管理</button></form>}
 <div className="toolbar"><div className="category-tabs">{[['all','全部资源'],['none','直接访问'],['account','需要服务账号']].map(([value,label])=><button key={value} className={filter===value?'selected':''} onClick={()=>setFilter(value)}>{label} · {services.filter(s=>value==='all'||s.accessRequirement===value).length}</button>)}</div><label className="search"><Search size={16}/><input aria-label="搜索公开资源" placeholder="搜索应用、分类…" value={search} onChange={e=>setSearch(e.target.value)}/></label></div>
 {error&&<div className="error" role="alert">{error}</div>}{loading&&<p role="status">正在加载公开资源…</p>}
 <section className="service-grid">{visible.map(s=><article className="service-card" key={s.id}><div className="tags"><span>{s.category||'应用'}</span><span>{s.accessRequirement==='none'?'直接访问':'需要服务账号'}</span></div><h3>{s.name}</h3><p>{s.description||'打开服务了解更多'}</p><div className="card-footer"><span>{s.accessRequirement==='none'?'无需账号':'由目标服务校验权限'}</span><a href={s.url} target="_blank" rel="noreferrer">打开服务 <ExternalLink size={13}/></a></div></article>)}</section>
 {!loading&&!error&&visible.length===0&&<div className="empty">{services.length?'没有匹配的公开资源。':'暂未发布公开资源，管理员可在服务编辑中设置公开展示。'}</div>}
 <footer>ALISA PORTAL · 公开应用目录</footer></main></div>;
}
