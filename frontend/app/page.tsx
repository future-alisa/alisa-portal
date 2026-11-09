'use client';
import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import { Network, Pencil, Activity, ArrowDownToLine, ArrowRight, Box, Boxes, ChevronRight, CircleHelp, ExternalLink, Layers3, LayoutDashboard, Plus, RefreshCw, Rocket, Search, Server, Settings2, ShieldCheck, Trash2, Users, X } from 'lucide-react';
import { type Service, type NetworkSettings, type AccessMode, emptyNetworkSettings, detectAccess, serviceEndpoint } from '../lib/network';
import { PublicCatalog } from '../components/public-catalog';
import { NetworkManager } from '../components/network-manager';
type Tenant = {
    id: string;
    name: string;
    namespace: string;
    cpu: number;
    memory: number;
};
type Deployment = {
    id: string;
    name: string;
    tenant: string;
    image: string;
    replicas: number;
    port: number;
    created: string;
    status: string;
    manifest: string;
};
type State = {
    networkSettings?: NetworkSettings;
    services: Service[];
    tenants: Tenant[];
    deployments: Deployment[];
    events: {
        message: string;
        time: string;
    }[];
};
const navigation = [{ label: '工作台', icon: LayoutDashboard }, { label: '服务资源', icon: Boxes }, { label: '资源配额', icon: Users }, { label: '部署中心', icon: Rocket }, { label: '资源监控', icon: Activity }, { label: '网络管理', icon: Network }];
const icons: Record<string, string> = { '网关': 'T', '存储': 'S', '数据库': 'D', '镜像仓库': 'R', '部署': 'A', '配置中心': 'N', '开发工具': 'G' };
export default function Home() {
    const [accessMode, setAccessMode] = useState<AccessMode>('auto');
    const [portalHost, setPortalHost] = useState('');
    const [editingService, setEditingService] = useState<Service | null>(null);
    useEffect(() => { setPortalHost(window.location.hostname); try {
        const saved = localStorage.getItem('alisa.network.accessMode');
        if (saved === 'internal' || saved === 'external')
            setAccessMode(saved);
    }
    catch { } }, []);
    const changeAccess = (value: AccessMode) => { setAccessMode(value); try {
        localStorage.setItem('alisa.network.accessMode', value);
    }
    catch { } };
    const [metrics, setMetrics] = useState<{
        mode: string;
        message: string;
        series: {
            key: string;
            label: string;
            value: number | null;
            unit: string;
        }[];
    }>({ mode: 'unconfigured', message: '尚未连接 Prometheus', series: [] });
    const [tab, setTab] = useState('工作台'), [state, setState] = useState<State>({ services: [], tenants: [], deployments: [], events: [] }), [loading, setLoading] = useState(true), [error, setError] = useState(''), [search, setSearch] = useState(''), [filter, setFilter] = useState('全部分类'), [modal, setModal] = useState(''), [busy, setBusy] = useState(false), [manifest, setManifest] = useState<Deployment | null>(null), [token, setToken] = useState(''), [tokenInput, setTokenInput] = useState(''), [updated, setUpdated] = useState('');
    const [cloudSession,setCloudSession]=useState<{user:string;csrfToken:string;canManage:boolean}|null>(null);
    const [authConfig,setAuthConfig]=useState<{enabled:boolean;allowApiToken:boolean}|null>(null);
    const [authError,setAuthError]=useState('');
    const credential=token || (cloudSession?.canManage ? cloudSession.csrfToken : '');
    const activeToken=useRef(credential);activeToken.current=credential;
    useEffect(()=>{
        const abort=new AbortController();
        const callbackError=new URL(window.location.href).searchParams.get('auth_error');
        const messages:Record<string,string>={forbidden:'当前账号无法登录门户，请确认你属于门户租户。',cancelled:'你已取消统一账号授权。',unavailable:'统一账号服务暂不可用，请稍后重试。'};
        if(callbackError){setAuthError(messages[callbackError]||'统一账号登录未完成。');const u=new URL(window.location.href);u.searchParams.delete('auth_error');window.history.replaceState(null,'',u.pathname+u.search+u.hash);}
        void Promise.all([fetch('/api/auth/config',{signal:abort.signal,cache:'no-store'}),fetch('/api/auth/session',{signal:abort.signal,cache:'no-store'})]).then(async([config,session])=>{
            if(!config.ok||!session.ok)throw new Error('无法确认统一账号状态，请刷新重试。');
            const c=await config.json();const data=await session.json();if(abort.signal.aborted)return;
            setAuthConfig(c);if(data.authenticated)setCloudSession({user:data.user,csrfToken:data.csrfToken,canManage:data.canManage===true});if(data.error)setAuthError(data.error);
        }).catch(e=>{if(!abort.signal.aborted)setAuthError(e.message);});
        return()=>abort.abort();
    },[]);
    const logout=async()=>{
        try {if(cloudSession){const r=await fetch('/api/auth/logout',{method:'POST',headers:{'X-CSRF-Token':cloudSession.csrfToken}});if(!r.ok)throw new Error('退出失败，请稍后重试');}}
        catch(e){setError(e instanceof Error?e.message:'退出失败');return;}
        setToken('');setCloudSession(null);setState({services:[],tenants:[],deployments:[],events:[]});setManifest(null);setModal('');
    };
    const request = useCallback(async (path: string, init?: RequestInit) => {
        if (!credential) throw new Error('请先登录管理账户');
        const res = await fetch('/api/' + path, { ...init, headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: 'Bearer ' + token } : {'X-CSRF-Token':cloudSession?.csrfToken||''}), ...init?.headers } });
        const data = await res.json();
        if (!res.ok){
            if((res.status===401||(res.status===403&&!cloudSession))&&activeToken.current===credential){setToken('');setCloudSession(null);setState({services:[],tenants:[],deployments:[],events:[]});setManifest(null);setModal('');setAuthError(data.error||'登录失效或权限不足');}
            throw new Error(data.error || '请求失败');
        }
        return data;
    }, [token,cloudSession,credential]);
    const refresh = useCallback(async () => {
        if (!credential) {
            setState({services:[],tenants:[],deployments:[],events:[]});
            setMetrics({mode:'unconfigured',message:'请先连接管理账户',series:[]});
            setManifest(null);setUpdated('');setError('');setLoading(false);return;
        }
        setLoading(true);
        try {
            const nextState=await request('state');
            if(activeToken.current!==credential)return;
            setState(nextState);
            try {
                const nextMetrics=await request('metrics');
                if(activeToken.current!==credential)return;
                setMetrics(nextMetrics);
            }
            catch {
                if(activeToken.current!==credential)return;
                setMetrics({ mode: 'error', message: '监控查询失败，请检查 Prometheus 连接', series: [] });
            }
            setError('');
            setUpdated(new Date().toLocaleTimeString('zh-CN'));
        }
        catch (e) {
            if(activeToken.current!==credential)return;
            setError(e instanceof Error ? e.message : '后端暂时不可用');
        }
        finally {
            if(activeToken.current===credential)setLoading(false);
        }
    }, [request,credential]);
    useEffect(() => { void refresh(); }, [refresh]);
    const submit = async (e: FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setBusy(true);
        setError('');
        const form = new FormData(e.currentTarget);
        const body: Record<string, string | number> = {};
        form.forEach((v, k) => body[k] = ['cpu', 'memory', 'replicas', 'port'].includes(k) ? Number(v) : String(v));
        try {
            setState(await request(modal === 'services' && editingService ? 'services/' + editingService.id : modal, { method: modal === 'services' && editingService ? 'PUT' : 'POST', body: JSON.stringify(body) }));
            setEditingService(null);
            setModal('');
            setUpdated(new Date().toLocaleTimeString('zh-CN'));
        }
        catch (e) {
            setError(e instanceof Error ? e.message : '操作失败');
        }
        finally {
            setBusy(false);
        }
    };
    const remove = async (s: Service) => {
        if (!confirm('确认从目录移除 ' + s.name + '？此操作不会删除实际基础设施。'))
            return;
        try {
            setState(await request('services/' + s.id, { method: 'DELETE' }));
        }
        catch (e) {
            setError(e instanceof Error ? e.message : '操作失败');
        }
    };
    const services = state.services.filter(s => (filter === '全部分类' || s.category === filter) && `${s.name} ${s.description}`.toLowerCase().includes(search.toLowerCase()));
    const tenantName = (id: string) => state.tenants.find(t => t.id === id)?.name || id;
    const open = (kind: string) => { setError(''); setEditingService(null); setModal(kind); };
    const editService = (service: Service) => { setError(''); setEditingService(service); setModal('services'); };
    const networkSettings = state.networkSettings || emptyNetworkSettings;
    const detected = detectAccess(portalHost, networkSettings);
    const accessScope = accessMode === 'auto' ? detected.scope : accessMode;
    const accessLabel = accessScope === 'internal' ? '内网' : '外网';
    const download = (d: Deployment) => { const url = URL.createObjectURL(new Blob([d.manifest], { type: 'text/yaml' })); const a = document.createElement('a'); a.href = url; a.download = d.name + '.yaml'; a.click(); URL.revokeObjectURL(url); };
    if (!credential) return <PublicCatalog member={cloudSession?.user} logout={() => void logout()} authConfig={authConfig} authError={authError || error} connect={value => { setState({services:[],tenants:[],deployments:[],events:[]}); setTab("工作台"); setToken(value); }}/>;
    return <div className="shell"><aside><a className="brand" href="/"><span className="brand-icon"><Layers3 size={24}/></span>alisa<span className="brand-dot">.</span></a><div className="workspace"><div className="avatar">A</div><div><strong>Alisa Workspace</strong><small>个人云 · 工作空间</small></div><ChevronRight size={15}/></div><div className="nav-caption">WORKSPACE</div><nav>{navigation.map(n => <button key={n.label} className={tab === n.label ? 'active' : ''} onClick={() => { setTab(n.label); setSearch(''); }}><n.icon size={19}/>{n.label}{n.label === '服务资源' && <span className="count">{state.services.length}</span>}</button>)}</nav><div className="aside-bottom"><div className="cluster-note"><span className="pulse"/> 本地管理模式<small>集群连接尚未配置</small></div><button onClick={() => void logout()}>退出管理</button><button onClick={() => open('settings')}><Settings2 size={18}/>连接设置</button><button onClick={() => open('help')}><CircleHelp size={18}/>使用指南</button><div className="profile"><div className="avatar dark">A</div><div><strong>{cloudSession?.user || 'Administrator'}</strong><small>{cloudSession ? '统一账号 · 门户管理员' : '管理员令牌连接'}</small></div></div></div></aside>
 <div className="main"><header><div className="breadcrumb">工作空间 <ChevronRight size={14}/><span>{tab}</span></div><div className="header-right"><span className="environment">{accessLabel}</span><span className="header-line"/><ShieldCheck size={17}/><span>管理控制台</span><div className="avatar small">A</div></div></header><main>
 <div className="page-heading"><div><div className="eyebrow">YOUR INFRASTRUCTURE, CONNECTED</div><h1>{tab === '工作台' ? '让基础设施，井然有序。' : tab}</h1><p>{({ '工作台': '在一个工作空间，连接服务、团队与每一次交付。', '服务资源': '统一登记基础设施服务，清晰掌握服务归属与访问入口。', '资源配额': '以命名空间组织团队，规划每个租户的资源配额。', '部署中心': '从容器镜像生成部署清单，交付至你的 GitOps 工作流。', '资源监控': '连接可观测性数据源，了解基础设施的真实运行状况。', '网络管理': '统一管理网络区域、内外网入口与服务访问规则。' } as Record<string, string>)[tab]}</p></div><div className="heading-actions"><button className="button" onClick={() => void refresh()} disabled={loading}><RefreshCw size={16} className={loading ? 'spin' : ''}/>刷新</button><button className="button primary" onClick={() => open(tab === '资源配额' ? 'tenants' : tab === '部署中心' ? 'deployments' : 'services')}><Plus size={17}/>{tab === '资源配额' ? '创建租户' : tab === '部署中心' ? '新建部署' : '登记服务'}</button></div></div>
 {error && <div className="error" role="alert">{error}<button onClick={() => setError('')} aria-label="关闭错误"><X size={16}/></button></div>}
 <div className="network-access"><div><Network size={17}/><strong>当前优先：{accessLabel}</strong><span>{accessMode === 'auto' ? detected.reason : '手动选择，已保存在此浏览器'}{portalHost ? ' · ' + portalHost : ''}</span></div><label>访问方式<select aria-label="服务访问方式" value={accessMode} onChange={e => changeAccess(e.target.value as AccessMode)}><option value="auto">自动判断</option><option value="external">外网优先</option><option value="internal">内网优先</option></select></label></div>
 <div className="mode-banner"><ShieldCheck size={17}/><span>{cloudSession ? '统一账号已连接' : '管理令牌已配置'}</span><span className="muted">{'· 服务运行状态需连接集群后核验'}</span><button onClick={() => open('settings')}>配置连接 <ArrowRight size={15}/></button></div>
 {(tab === '工作台' || tab === '资源监控') && <section className="stats">{[{ label: '已登记服务', value: state.services.length, unit: '个', icon: Boxes, note: '统一服务目录', color: 'blue' }, { label: '工作空间租户', value: state.tenants.length, unit: '个', icon: Users, note: '独立命名空间规划', color: 'purple' }, { label: '部署申请', value: state.deployments.length, unit: '项', icon: Rocket, note: '待 GitOps 发布', color: 'orange' }, { label: '集群监控', value: metrics.mode === 'connected' ? '已连接' : '--', unit: '', icon: Activity, note: metrics.mode === 'connected' ? 'Prometheus 实时查询' : '等待连接 Prometheus', color: 'green' }].map(s => <article className="stat" key={s.label}><div className="stat-top">{s.label}<span className={'icon-box ' + s.color}><s.icon size={18}/></span></div><div className="stat-value">{s.value}<span>{s.unit}</span></div><div className="stat-note"><span className={'tiny-dot ' + s.color}/>{s.note}</div></article>)}</section>}
 {(tab === '工作台' || tab === '服务资源') && <><div className="section-heading"><div><h2>服务资源 <span>{state.services.length}</span></h2><p>你的基础设施，从这里连接</p></div>{tab === '工作台' && <button className="text-button" onClick={() => setTab('服务资源')}>查看全部资源 <ArrowRight size={15}/></button>}</div><div className="toolbar"><div className="category-tabs">{['全部分类', '网关', '存储', '数据库', '开发工具'].map(c => <button key={c} onClick={() => setFilter(c)} className={filter === c ? 'selected' : ''}>{c}</button>)}</div><label className="search"><Search size={16}/><input aria-label="搜索服务" placeholder="搜索服务名称、描述…" value={search} onChange={e => setSearch(e.target.value)}/></label></div><section className="service-grid">{services.map((s, i) => <article className="service-card" key={s.id}><div className="service-top"><div className={'service-icon color-' + (i % 5)}>{icons[s.category] || <Box size={23}/>}</div><span className="status"><span />待核验</span></div><h3>{s.name}</h3><p>{s.description || '暂无服务说明'}</p><div className="tags"><span>{s.category || '其他'}</span><span>{tenantName(s.tenant)}</span><span>{s.visibility === "public" ? "公开展示" : "仅管理员"}</span><span>{s.accessRequirement === "none" ? "直接访问" : "需要服务账号"}</span></div><div className="service-endpoints">{(['internal', 'external'] as const).map(scope => serviceEndpoint(s, scope) ? <a key={scope} href={serviceEndpoint(s, scope)} target="_blank" rel="noreferrer" title={serviceEndpoint(s, scope)}>{scope === 'internal' ? '内网' : '外网'} <ExternalLink size={11}/></a> : <span key={scope}>{scope === 'internal' ? '内网' : '外网'}未配置</span>)}</div><div className="card-footer"><span><Server size={13}/>{networkSettings.networks.find(n => n.id === s.networkId)?.name || '未归类网络'}</span><div>{serviceEndpoint(s, accessScope) ? <a href={serviceEndpoint(s, accessScope)} target="_blank" rel="noreferrer" aria-label={'打开 ' + s.name + '（' + accessLabel + '）'}>打开{accessLabel} <ExternalLink size={13}/></a> : <span className="unconfigured">未配置{accessLabel}入口</span>}<button aria-label={'编辑 ' + s.name} onClick={() => editService(s)}><Pencil size={14}/></button><button aria-label={'移除 ' + s.name} onClick={() => void remove(s)}><Trash2 size={14}/></button></div></div></article>)}</section>{!loading && services.length === 0 && <div className="empty">没有匹配的服务，试试其他搜索条件或登记一个服务。</div>}</>}
 {tab === '网络管理' && <NetworkManager settings={networkSettings} services={state.services} editService={editService} save={async (settings) => { setState(await request('network-settings', { method: 'PUT', body: JSON.stringify(settings) })); }}/>}
 {tab === '资源配额' && <section className="panel table-panel"><div className="section-heading"><h2>租户与配额</h2><span className="muted">配额为门户规划值，尚未同步集群</span></div><div className="table-scroll"><table><thead><tr><th>租户名称</th><th>命名空间</th><th>CPU 配额</th><th>内存配额</th><th>服务数</th><th>状态</th></tr></thead><tbody>{state.tenants.map(t => <tr key={t.id}><td><span className="table-avatar">{t.name.slice(0, 1)}</span><strong>{t.name}</strong></td><td><code>{t.namespace}</code></td><td>{t.cpu} 核</td><td>{t.memory} GiB</td><td>{state.services.filter(s => s.tenant === t.id).length}</td><td><span className="badge">本地登记</span></td></tr>)}</tbody></table></div></section>}
 {tab === '部署中心' && <section className="panel"><div className="section-heading"><h2>部署申请</h2><span className="badge">Manifest → Git → Argo CD</span></div>{!state.deployments.length ? <div className="empty"><Rocket size={36}/><h3>开始你的第一次交付</h3><p>选择租户和容器镜像，生成 Deployment 与 Service 清单。</p><button className="button primary" onClick={() => open('deployments')}><Plus size={16}/>新建部署</button></div> : state.deployments.map(d => <div className="deployment-row" key={d.id}><span className="icon-box purple"><Rocket size={20}/></span><div><strong>{d.name}</strong><p>{d.image} · {tenantName(d.tenant)} · {d.replicas} 副本</p><small>{new Date(d.created).toLocaleString('zh-CN')}</small></div><span className="badge">清单待发布</span><button className="button" onClick={() => setManifest(d)}>查看清单</button><button className="button" onClick={() => download(d)}><ArrowDownToLine size={15}/>下载</button></div>)}</section>}
 {tab === '资源监控' && <><section className="monitor-grid">{['CPU 使用率', '内存使用率', '存储使用率', '运行中 Pod'].map((label, index) => <article className="panel monitor" key={label}><div className="section-heading"><h2>{label}</h2><span className="badge">{metrics.mode === 'connected' ? '实时查询' : '未连接'}</span></div><div className="monitor-placeholder"><Activity size={30}/><strong>{metrics.series[index]?.value != null ? metrics.series[index].value.toFixed(1) + ' ' + metrics.series[index].unit : '暂无采集数据'}</strong><p>{metrics.mode === 'connected' ? '集群汇总 · 缺少 exporter 时指标为空' : metrics.message}</p></div></article>)}</section><div className="panel"><h2>可观测性接入规划</h2><p className="muted">Prometheus 采集指标 · Grafana 可视化 · Loki 聚合日志 · Alertmanager 告警路由</p><p className="muted">已支持 Prometheus 即时查询；Go 服务端配置 PROMETHEUS_URL 后点击刷新。需要 node-exporter 和 kube-state-metrics。Grafana、日志及告警连接器待接入。</p></div></>}
 {tab === '工作台' && <section className="bottom-grid"><article className="panel"><div className="section-heading"><h2>交付工作流</h2><span className="badge">GITOPS</span></div><div className="workflow">{[{ i: Box, t: '登记服务', s: '建立资源与归属' }, { i: Users, t: '规划租户', s: '配置命名空间配额' }, { i: Rocket, t: '生成清单', s: '通过 GitOps 交付' }].map((w, i) => <button key={w.t} onClick={() => open(['services', 'tenants', 'deployments'][i])}><span className="step-icon"><w.i size={22}/></span><strong>{w.t}</strong><small>{w.s}</small>{i < 2 && <ChevronRight className="step-arrow" size={17}/>}</button>)}</div></article><article className="panel"><div className="section-heading"><h2>最近动态</h2><span className="muted">操作记录</span></div><div className="events">{state.events.length ? state.events.slice(0, 3).map((e, i) => <div className="event" key={i}><span className="event-dot"/><span>{e.message}<small>{new Date(e.time).toLocaleString('zh-CN')}</small></span></div>) : <div className="event"><span className="event-dot"/><span>欢迎使用 Alisa Portal<small>登记服务，开启你的基础设施工作空间</small></span></div>}</div></article></section>}
 <footer><span>ALISA PORTAL <span className="footer-dot">/</span> 为你的基础设施而构建</span><span>{loading ? '正在同步…' : updated ? '最近同步 ' + updated : '等待连接后端'} <span className="footer-dot">·</span> v0.1.0</span></footer>
 </main></div>
 {modal && <div className="overlay" onMouseDown={e => {
                if (e.target === e.currentTarget && !busy)
                    setModal('');
            }}><section className="dialog" role="dialog" aria-modal="true" aria-labelledby="dialog-title"><div className="section-heading"><h2 id="dialog-title">{({ services: editingService ? '编辑服务与内外网地址' : '登记服务资源', tenants: '创建租户', deployments: '新建部署申请', settings: '连接设置', help: '使用指南' } as Record<string, string>)[modal]}</h2><button aria-label="关闭" disabled={busy} onClick={() => setModal('')}><X size={20}/></button></div>{error && <div className="error" role="alert">{error}</div>}
 {['services', 'tenants', 'deployments'].includes(modal) && <form key={modal + (editingService?.id || 'new')} onSubmit={submit}>{modal === 'services' && <><label>服务名称<input name="name" defaultValue={editingService?.name} required maxLength={80} placeholder="例如：Redis"/></label><label>资源分类<select name="category" defaultValue={editingService?.category}>{Array.from(new Set(['网关', '存储', '数据库', '镜像仓库', '部署', '配置中心', '开发工具', '其他', ...state.services.map(s => s.category).filter(Boolean)])).map(c => <option key={c}>{c}</option>)}</select></label><label>外网地址（选填）<input name="externalUrl" type="url" defaultValue={editingService?.externalUrl || editingService?.url || ''} maxLength={2048} placeholder="https://your-service.example.com"/></label><label>内网地址（选填）<input name="internalUrl" type="url" defaultValue={editingService?.internalUrl || ''} maxLength={2048} placeholder="https://service.example.local"/></label><label>网络区域<select name="networkId" defaultValue={editingService?.networkId || ''}><option value="">未归类</option>{networkSettings.networks.map(n => <option key={n.id} value={n.id}>{n.name}</option>)}</select></label><p className="form-note">外网访问默认打开外网入口，内网访问默认打开内网入口；可手动切换。地址缺失时不会静默跳到另一网络。</p><label>门户展示范围<select name="visibility" defaultValue={editingService?.visibility || "private"}><option value="private">仅管理员可见</option><option value="public">公开目录可见</option></select></label><label>目标服务访问要求<select name="accessRequirement" defaultValue={editingService?.accessRequirement || "account"}><option value="none">直接访问（无需账号）</option><option value="account">需要服务账号 / 相应权限</option></select></label><label>公开说明<textarea name="publicDescription" defaultValue={editingService?.publicDescription || ""} maxLength={500} placeholder="仅此说明展示给访客，请勿填写内部信息"/></label><p className="form-note">公开目录仅展示名称、分类、公开说明与外网地址。目标服务仍执行自己的账号与权限校验。</p><label>服务说明<textarea name="description" defaultValue={editingService?.description} maxLength={500} placeholder="简要描述服务用途"/></label></>}{modal === 'tenants' && <><label>租户名称<input name="name" required maxLength={80} placeholder="例如：研发团队"/></label><label>命名空间<input name="namespace" required pattern="[a-z][a-z0-9-]{0,61}[a-z0-9]" placeholder="例如：team-dev"/></label><div className="form-row"><label>CPU 配额（核）<input name="cpu" type="number" min={1} max={1000} defaultValue={4} required/></label><label>内存配额（GiB）<input name="memory" type="number" min={1} max={4096} defaultValue={8} required/></label></div><p className="form-note">此操作仅在门户登记租户，不会在集群创建命名空间或 ResourceQuota。</p></>}{modal === 'deployments' && <><label>应用名称<input name="name" required pattern="[a-z][a-z0-9-]{0,61}[a-z0-9]" placeholder="my-application"/></label><label>容器镜像<input name="image" required placeholder="registry.example.com/app:1.0.0"/></label><div className="form-row"><label>副本数<input name="replicas" type="number" min={1} max={20} defaultValue={1} required/></label><label>容器端口<input name="port" type="number" min={1} max={65535} defaultValue={8080} required/></label></div><p className="form-note">生成清单后请审阅并提交 Git 仓库，由 Argo CD 同步。每副本按上限 250m CPU / 256Mi 预留门户配额。</p></>}{modal !== 'tenants' && <label>所属租户<select name="tenant" defaultValue={editingService?.tenant} required>{state.tenants.map(t => <option key={t.id} value={t.id}>{t.name} · {t.namespace}</option>)}</select></label>}<div className="dialog-actions"><button className="button" type="button" disabled={busy} onClick={() => setModal('')}>取消</button><button className="button primary" disabled={busy}>{busy ? '保存中…' : modal === 'deployments' ? '生成部署清单' : '保存'}</button></div></form>}
 {modal === 'settings' && cloudSession && <><p>当前统一账号：{cloudSession.user}</p><p className="form-note">管理权限由 Alisa Cloud 授权中心实时校验。退出仅结束门户会话。</p><button className="button" onClick={() => void logout()}>退出门户</button></>}
 {modal === 'settings' && !cloudSession && <><p className="muted">前端通过 Next.js 同源代理连接 Go API。服务端 API_URL 默认指向本地 8080 端口。</p><label>管理员 API 令牌<input type="password" autoComplete="off" value={tokenInput} onChange={e => setTokenInput(e.target.value)} placeholder="与后端 API_TOKEN 保持一致"/></label><p className="form-note">令牌仅保存在当前页面内存中，刷新页面后需重新输入。Prometheus 在 Go 服务端通过 PROMETHEUS_URL 配置；集群和 Argo CD 连接器尚未实现。</p><button className="button primary" onClick={() => { setToken(tokenInput.trim()); setTokenInput(''); setModal(''); }}>应用连接设置</button></>}
 {modal === 'help' && <div className="help"><p>1. 在服务资源中登记服务地址，按团队归属管理基础设施。</p><p>2. 在资源配额中规划命名空间和配额。</p><p>3. 在部署中心填写容器镜像，下载并审阅 Kubernetes 清单。</p><p>4. 将清单提交到 GitOps 仓库，由 Argo CD 进行实际部署。</p><p>服务状态均为待核验。Prometheus 需配置数据源，统一登录需在服务端配置 Alisa Cloud，集群操作需要后续接入，详细说明见项目 README。</p></div>}</section></div>}
 {manifest && <div className="overlay"><section className="dialog wide" role="dialog" aria-modal="true" aria-label="部署清单"><div className="section-heading"><h2>{manifest.name} · Kubernetes 清单</h2><button aria-label="关闭清单" onClick={() => setManifest(null)}><X size={20}/></button></div><pre>{manifest.manifest}</pre><button className="button primary" onClick={() => download(manifest)}><ArrowDownToLine size={16}/>下载 YAML</button></section></div>}
 </div>;
}
