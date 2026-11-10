'use client';
import { useCallback, useEffect, useRef, useState } from 'react';
import { Network, Activity, ArrowRight, Box, Boxes, ChevronRight, CircleHelp, ExternalLink, Layers3, LayoutDashboard, RefreshCw, Search, Server, Settings2, ShieldCheck, X } from 'lucide-react';
import { type Service, type NetworkSettings, type AccessMode, emptyNetworkSettings, detectAccess, serviceEndpoint } from '../lib/network';
import { PublicCatalog } from '../components/public-catalog';
type Tenant = {
    id: string;
    name: string;
    namespace: string;
};
type State = {
    networkSettings?: NetworkSettings;
    services: Service[];
    tenants: Tenant[];
    events: {
        message: string;
        time: string;
    }[];
};
const navigation = [{ label: '工作台', icon: LayoutDashboard }, { label: '服务资源', icon: Boxes }, { label: '资源监控', icon: Activity }];
const icons: Record<string, string> = { '网关': 'T', '存储': 'S', '数据库': 'D', '镜像仓库': 'R', '部署': 'A', '配置中心': 'N', '开发工具': 'G' };
export default function Home() {
    const [accessMode, setAccessMode] = useState<AccessMode>('auto');
    const [portalHost, setPortalHost] = useState('');
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
    const [tab, setTab] = useState('工作台'), [state, setState] = useState<State>({ services: [], tenants: [], events: [] }), [loading, setLoading] = useState(true), [error, setError] = useState(''), [search, setSearch] = useState(''), [filter, setFilter] = useState('全部分类'), [modal, setModal] = useState(''), [token, setToken] = useState(''), [tokenInput, setTokenInput] = useState(''), [updated, setUpdated] = useState('');
    const [cloudSession,setCloudSession]=useState<{csrfToken:string;canManage:boolean}|null>(null);
    const [authConfig,setAuthConfig]=useState<{enabled:boolean;allowApiToken:boolean}|null>(null);
    const [authError,setAuthError]=useState('');
    const [checkingSession, setCheckingSession] = useState(true);
    const [sessionCheckError, setSessionCheckError] = useState('');
    const [sessionCheckAttempt, setSessionCheckAttempt] = useState(0);
    const credential=token || (cloudSession?.canManage ? cloudSession.csrfToken : '');
    const activeToken=useRef(credential);activeToken.current=credential;
    useEffect(()=>{
        const abort=new AbortController();
        const timeout = window.setTimeout(() => {
            abort.abort();
            setSessionCheckError('确认登录状态超时。');
            setCheckingSession(false);
        }, 8000);
        const callbackError=new URL(window.location.href).searchParams.get('auth_error');
        const messages:Record<string,string>={forbidden:'当前账号无法登录门户，请确认你属于门户租户。',cancelled:'你已取消统一账号授权。',unavailable:'统一账号服务暂不可用，请稍后重试。'};
        if(callbackError){setAuthError(messages[callbackError]||'统一账号登录未完成。');const u=new URL(window.location.href);u.searchParams.delete('auth_error');window.history.replaceState(null,'',u.pathname+u.search+u.hash);}
        void Promise.all([fetch('/api/auth/config',{signal:abort.signal,cache:'no-store'}),fetch('/api/auth/session',{signal:abort.signal,cache:'no-store'})]).then(async([config,session])=>{
            if(!config.ok||!session.ok)throw new Error('无法确认统一账号状态。');
            const c=await config.json();const data=await session.json();if(abort.signal.aborted)return;
            setAuthConfig(c);if(data.authenticated)setCloudSession({csrfToken:data.csrfToken,canManage:data.canManage===true});if(data.error)setAuthError(data.error);
        }).catch(() => {
            if (!abort.signal.aborted) setSessionCheckError('暂时无法确认登录状态。');
        }).finally(() => {
            window.clearTimeout(timeout);
            if (!abort.signal.aborted) setCheckingSession(false);
        });
        return () => {
            window.clearTimeout(timeout);
            abort.abort();
        };
    },[sessionCheckAttempt]);
    const logout=async()=>{
        try {if(cloudSession){const r=await fetch('/api/auth/logout',{method:'POST',headers:{'X-CSRF-Token':cloudSession.csrfToken}});if(!r.ok)throw new Error('退出失败，请稍后重试');}}
        catch(e){setError(e instanceof Error?e.message:'退出失败');return;}
        setToken('');setCloudSession(null);setState({services:[],tenants:[],events:[]});setModal('');
    };
    const request = useCallback(async (path: string, init?: RequestInit) => {
        if (!credential) throw new Error('请先登录管理账户');
        const res = await fetch('/api/' + path, { ...init, headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: 'Bearer ' + token } : {'X-CSRF-Token':cloudSession?.csrfToken||''}), ...init?.headers } });
        const data = await res.json();
        if (!res.ok){
            if((res.status===401||(res.status===403&&!cloudSession))&&activeToken.current===credential){setToken('');setCloudSession(null);setState({services:[],tenants:[],events:[]});setModal('');setAuthError(data.error||'登录失效或权限不足');}
            throw new Error(data.error || '请求失败');
        }
        return data;
    }, [token,cloudSession,credential]);
    const refresh = useCallback(async () => {
        if (!credential) {
            setState({services:[],tenants:[],events:[]});
            setMetrics({mode:'unconfigured',message:'请先连接管理账户',series:[]});
            setUpdated('');setError('');setLoading(false);return;
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
    const services = state.services.filter(s => (filter === '全部分类' || s.category === filter) && `${s.name} ${s.description}`.toLowerCase().includes(search.toLowerCase()));
    const tenantName = (id: string) => state.tenants.find(t => t.id === id)?.name || id;
    const open = (kind: string) => { setError(''); setModal(kind); };
    const networkSettings = state.networkSettings || emptyNetworkSettings;
    const detected = detectAccess(portalHost, networkSettings);
    const accessScope = accessMode === 'auto' ? detected.scope : accessMode;
    const accessLabel = accessScope === 'internal' ? '内网' : '外网';
    if (checkingSession) return <main className="session-loading"><div className="login-loading" role="status"><RefreshCw size={16} className="spin" aria-hidden="true"/>正在确认登录状态…</div></main>;
    if (sessionCheckError) return <main className="session-loading"><div className="login-loading"><span role="alert">{sessionCheckError}</span><button className="button" onClick={() => {
        setSessionCheckError('');
        setCheckingSession(true);
        setSessionCheckAttempt(attempt => attempt + 1);
    }}>重试</button></div></main>;
    if (!credential) return <PublicCatalog signedIn={cloudSession !== null} logout={() => void logout()} authConfig={authConfig} authError={authError || error} connect={value => { setState({services:[],tenants:[],events:[]}); setTab("工作台"); setToken(value); }}/>;
    return <div className="shell"><aside><a className="brand" href="/"><span className="brand-icon"><Layers3 size={24}/></span>alisa<span className="brand-dot">.</span></a><div className="workspace"><div className="avatar">A</div><div><strong>Alisa Workspace</strong><small>个人云 · 工作空间</small></div><ChevronRight size={15}/></div><div className="nav-caption">WORKSPACE</div><nav>{navigation.map(n => <button key={n.label} className={tab === n.label ? 'active' : ''} onClick={() => { setTab(n.label); setSearch(''); }}><n.icon size={19}/>{n.label}{n.label === '服务资源' && <span className="count">{state.services.length}</span>}</button>)}</nav><div className="aside-bottom"><div className="cluster-note"><span className="pulse"/> 本地管理模式<small>集群连接尚未配置</small></div><button onClick={() => void logout()}>退出管理</button><button onClick={() => open('settings')}><Settings2 size={18}/>连接设置</button><button onClick={() => open('help')}><CircleHelp size={18}/>使用指南</button><div className="profile"><div className="avatar dark">A</div><div><strong>{cloudSession ? '门户管理员' : 'Administrator'}</strong><small>{cloudSession ? 'Alisa 统一账号' : '管理员令牌连接'}</small></div></div></div></aside>
 <div className="main"><header><div className="breadcrumb">工作空间 <ChevronRight size={14}/><span>{tab}</span></div><div className="header-right"><span className="environment">{accessLabel}</span><span className="header-line"/><ShieldCheck size={17}/><span>管理控制台</span><div className="avatar small">A</div></div></header><main>
 <div className="page-heading"><div><div className="eyebrow">YOUR INFRASTRUCTURE, CONNECTED</div><h1>{tab === '工作台' ? '让基础设施，井然有序。' : tab}</h1><p>{({ '工作台': '查找和打开你需要的服务。', '服务资源': '统一登记基础设施服务，清晰掌握服务归属与访问入口。', '资源监控': '连接可观测性数据源，了解基础设施的真实运行状况。' } as Record<string, string>)[tab]}</p></div><div className="heading-actions"><button className="button" onClick={() => void refresh()} disabled={loading}><RefreshCw size={16} className={loading ? 'spin' : ''}/>刷新</button><a className="button primary" href="/admin/portal-services">管理服务目录 <ExternalLink size={15}/></a></div></div>
 {error && <div className="error" role="alert">{error}<button onClick={() => setError('')} aria-label="关闭错误"><X size={16}/></button></div>}
 <div className="network-access"><div><Network size={17}/><strong>当前优先：{accessLabel}</strong><span>{accessMode === 'auto' ? detected.reason : '手动选择，已保存在此浏览器'}{portalHost ? ' · ' + portalHost : ''}</span></div><label>访问方式<select aria-label="服务访问方式" value={accessMode} onChange={e => changeAccess(e.target.value as AccessMode)}><option value="auto">自动判断</option><option value="external">外网优先</option><option value="internal">内网优先</option></select></label></div>
 <div className="mode-banner"><ShieldCheck size={17}/><span>{cloudSession ? '统一账号已连接' : '管理令牌已配置'}</span><span className="muted">{'· 服务运行状态需连接集群后核验'}</span><button onClick={() => open('settings')}>配置连接 <ArrowRight size={15}/></button></div>
 {(tab === '工作台' || tab === '资源监控') && <section className="stats">{[{ label: '已登记服务', value: state.services.length, unit: '个', icon: Boxes, note: '统一服务目录', color: 'blue' }, { label: '集群监控', value: metrics.mode === 'connected' ? '已连接' : '--', unit: '', icon: Activity, note: metrics.mode === 'connected' ? 'Prometheus 实时查询' : '等待连接 Prometheus', color: 'green' }].map(s => <article className="stat" key={s.label}><div className="stat-top">{s.label}<span className={'icon-box ' + s.color}><s.icon size={18}/></span></div><div className="stat-value">{s.value}<span>{s.unit}</span></div><div className="stat-note"><span className={'tiny-dot ' + s.color}/>{s.note}</div></article>)}</section>}
 {(tab === '工作台' || tab === '服务资源') && <><div className="section-heading"><div><h2>服务资源 <span>{state.services.length}</span></h2><p>你的基础设施，从这里连接</p></div>{tab === '工作台' && <button className="text-button" onClick={() => setTab('服务资源')}>查看全部资源 <ArrowRight size={15}/></button>}</div><div className="toolbar"><div className="category-tabs">{['全部分类', '网关', '存储', '数据库', '开发工具'].map(c => <button key={c} onClick={() => setFilter(c)} className={filter === c ? 'selected' : ''}>{c}</button>)}</div><label className="search"><Search size={16}/><input aria-label="搜索服务" placeholder="搜索服务名称、描述…" value={search} onChange={e => setSearch(e.target.value)}/></label></div><section className="service-grid">{services.map((s, i) => <article className="service-card" key={s.id}><div className="service-top"><div className={'service-icon color-' + (i % 5)}>{icons[s.category] || <Box size={23}/>}</div><span className="status"><span />待核验</span></div><h3>{s.name}</h3><p>{s.description || '暂无服务说明'}</p><div className="tags"><span>{s.category || '其他'}</span><span>{tenantName(s.tenant)}</span><span>{s.visibility === "public" ? "公开展示" : "仅管理员"}</span><span>{s.accessRequirement === "none" ? "直接访问" : "需要服务账号"}</span></div><div className="service-endpoints">{(['internal', 'external'] as const).map(scope => serviceEndpoint(s, scope) ? <a key={scope} href={serviceEndpoint(s, scope)} target="_blank" rel="noreferrer" title={serviceEndpoint(s, scope)}>{scope === 'internal' ? '内网' : '外网'} <ExternalLink size={11}/></a> : <span key={scope}>{scope === 'internal' ? '内网' : '外网'}未配置</span>)}</div><div className="card-footer"><span><Server size={13}/>{networkSettings.networks.find(n => n.id === s.networkId)?.name || '未归类网络'}</span><div>{serviceEndpoint(s, accessScope) ? <a href={serviceEndpoint(s, accessScope)} target="_blank" rel="noreferrer" aria-label={'打开 ' + s.name + '（' + accessLabel + '）'}>打开{accessLabel} <ExternalLink size={13}/></a> : <span className="unconfigured">未配置{accessLabel}入口</span>}</div></div></article>)}</section>{!loading && services.length === 0 && <div className="empty">没有匹配的服务，试试其他搜索条件或登记一个服务。</div>}</>}
 {tab === '资源监控' && <><section className="monitor-grid">{['CPU 使用率', '内存使用率', '存储使用率', '运行中 Pod'].map((label, index) => <article className="panel monitor" key={label}><div className="section-heading"><h2>{label}</h2><span className="badge">{metrics.mode === 'connected' ? '实时查询' : '未连接'}</span></div><div className="monitor-placeholder"><Activity size={30}/><strong>{metrics.series[index]?.value != null ? metrics.series[index].value.toFixed(1) + ' ' + metrics.series[index].unit : '暂无采集数据'}</strong><p>{metrics.mode === 'connected' ? '集群汇总 · 缺少 exporter 时指标为空' : metrics.message}</p></div></article>)}</section><div className="panel"><h2>可观测性接入规划</h2><p className="muted">Prometheus 采集指标 · Grafana 可视化 · Loki 聚合日志 · Alertmanager 告警路由</p><p className="muted">已支持 Prometheus 即时查询；Go 服务端配置 PROMETHEUS_URL 后点击刷新。需要 node-exporter 和 kube-state-metrics。Grafana、日志及告警连接器待接入。</p></div></>}
 {tab === '工作台' && <section className="bottom-grid"><article className="panel"><div className="section-heading"><h2>最近动态</h2><span className="muted">操作记录</span></div><div className="events">{state.events.length ? state.events.slice(0, 3).map((e, i) => <div className="event" key={i}><span className="event-dot"/><span>{e.message}<small>{new Date(e.time).toLocaleString('zh-CN')}</small></span></div>) : <div className="event"><span className="event-dot"/><span>欢迎使用 Alisa Portal<small>登记服务，开启你的基础设施工作空间</small></span></div>}</div></article></section>}
 <footer><span>ALISA PORTAL <span className="footer-dot">/</span> 为你的基础设施而构建</span><span>{loading ? '正在同步…' : updated ? '最近同步 ' + updated : '等待连接后端'} <span className="footer-dot">·</span> v0.1.0</span></footer>
 </main></div>
 {modal && <div className="overlay" onMouseDown={e => {
                if (e.target === e.currentTarget)
                    setModal('');
            }}><section className="dialog" role="dialog" aria-modal="true" aria-labelledby="dialog-title"><div className="section-heading"><h2 id="dialog-title">{({ settings: '连接设置', help: '使用指南' } as Record<string, string>)[modal]}</h2><button aria-label="关闭" onClick={() => setModal('')}><X size={20}/></button></div>{error && <div className="error" role="alert">{error}</div>}
 {modal === 'settings' && cloudSession && <><p>当前使用 Alisa 统一账号登录。</p><p className="form-note">管理权限由 Alisa Cloud 授权中心实时校验。退出仅结束门户会话。</p><button className="button" onClick={() => void logout()}>退出门户</button></>}
 {modal === 'settings' && !cloudSession && <><p className="muted">前端通过 Next.js 同源代理连接 Go API。服务端 API_URL 默认指向本地 8080 端口。</p><label>管理员 API 令牌<input type="password" autoComplete="off" value={tokenInput} onChange={e => setTokenInput(e.target.value)} placeholder="与后端 API_TOKEN 保持一致"/></label><p className="form-note">令牌仅保存在当前页面内存中，刷新页面后需重新输入。Prometheus 在 Go 服务端通过 PROMETHEUS_URL 配置；集群和 Argo CD 连接器尚未实现。</p><button className="button primary" onClick={() => { setToken(tokenInput.trim()); setTokenInput(''); setModal(''); }}>应用连接设置</button></>}
 {modal === 'help' && <div className="help"><p>在服务资源中查找并打开应用。管理员可在 Alisa Admin 编辑服务目录。</p><p>服务状态均为待核验。Prometheus 需配置数据源，统一登录需在服务端配置 Alisa Cloud，集群操作需要后续接入，详细说明见项目 README。</p></div>}</section></div>}
 </div>;
}
