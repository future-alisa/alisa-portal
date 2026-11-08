'use client';
import { useEffect, useState, type FormEvent } from 'react';
import { Network, Plus, Pencil, Trash2, Globe, Server } from 'lucide-react';
import { type NetworkSettings, type NetworkZone, type Service, serviceEndpoint } from '../lib/network';
type Props = {
    settings: NetworkSettings;
    services: Service[];
    save: (settings: NetworkSettings) => Promise<void>;
    editService: (service: Service) => void;
};
const blankZone: NetworkZone = { id: '', name: '', scope: 'internal', cidr: '', gateway: '', dns: '', description: '' };
const scopeLabels = { internal: '内网', external: '外网', tunnel: '穿透 / VPN' };
export function NetworkManager({ settings, services, save, editService }: Props) {
    const [internalHosts, setInternalHosts] = useState(settings.internalHosts.join('\n'));
    const [externalHosts, setExternalHosts] = useState(settings.externalHosts.join('\n'));
    const [zone, setZone] = useState<NetworkZone | null>(null), [busy, setBusy] = useState(false), [error, setError] = useState(''), [success, setSuccess] = useState('');
    useEffect(() => { setInternalHosts(settings.internalHosts.join('\n')); setExternalHosts(settings.externalHosts.join('\n')); }, [settings]);
    const apply = async (next: NetworkSettings) => { setBusy(true); setError(''); setSuccess(''); try {
        await save(next);
        setZone(null);
        setSuccess('网络配置已保存');
    }
    catch (e) {
        setError(e instanceof Error ? e.message : '保存失败');
    }
    finally {
        setBusy(false);
    } };
    const hosts = (value: string) => value.split(/[\s,，]+/).map(x => x.trim()).filter(Boolean);
    const saveRules = (e: FormEvent) => { e.preventDefault(); void apply({ ...settings, internalHosts: hosts(internalHosts), externalHosts: hosts(externalHosts) }); };
    const saveZone = (e: FormEvent) => { e.preventDefault(); if (!zone)
        return; const networks = zone.id ? settings.networks.map(n => n.id === zone.id ? zone : n) : [...settings.networks, zone]; void apply({ ...settings, networks }); };
    return <div className="network-manager">
  {error && <div role="alert" className="error">{error}</div>}{success && <div role="status" className="network-success">{success}</div>}
  <section className="stats network-stats">{[{ label: '内网地址', value: services.filter(s => serviceEndpoint(s, 'internal')).length, icon: Server }, { label: '外网地址', value: services.filter(s => serviceEndpoint(s, 'external')).length, icon: Globe }, { label: '双地址服务', value: services.filter(s => serviceEndpoint(s, 'internal') && serviceEndpoint(s, 'external')).length, icon: Network }, { label: '网络区域', value: settings.networks.length, icon: Network }].map(s => <article key={s.label} className="stat"><div className="stat-top">{s.label}<s.icon size={18}/></div><div className="stat-value">{s.value}</div></article>)}</section>
  <section className="panel network-rules"><div className="section-heading"><div><h2>门户入口与默认选址</h2><p>填写访问门户本身的主机名，不是服务的主机名；每行一个，不含协议、端口或路径。</p></div><span className="badge">按入口域名判断</span></div>
   <form onSubmit={saveRules}><div className="network-columns"><label>内网门户主机<textarea value={internalHosts} onChange={e => setInternalHosts(e.target.value)} placeholder={'portal.example.internal\nportal.example.local'}/></label><label>外网门户主机<textarea value={externalHosts} onChange={e => setExternalHosts(e.target.value)} placeholder="portal.example.com"/></label></div><p className="form-note">显式规则优先；私有 IP、localhost 和 .local 默认按内网处理，其他主机默认外网。同一个公网域名在局域网中访问，仍默认外网，请手动切换。分视 DNS 使用同一域名时，应使用同一服务 URL，由 DNS 负责内外网解析。</p><button className="button primary" disabled={busy}>{busy ? '保存中…' : '保存入口规则'}</button></form>
  </section>
  <section className="panel"><div className="section-heading"><div><h2>网络区域</h2><p>登记 LAN、容器网络、外网入口、穿透或 VPN，统一查看网段、网关和 DNS。</p></div><button className="button" disabled={busy} onClick={() => { setZone({ ...blankZone }); setError(''); }}><Plus size={15}/>新增网络</button></div>
   {!settings.networks.length ? <div className="empty">尚未登记网络区域。添加后，可在服务编辑中关联区域。</div> : <div className="network-zone-grid">{settings.networks.map(n => <article key={n.id} className="network-zone"><div className="section-heading"><strong>{n.name}</strong><span className="badge">{scopeLabels[n.scope]}</span></div><dl><dt>网段</dt><dd>{n.cidr || '未配置'}</dd><dt>网关 / 入口</dt><dd>{n.gateway || '未配置'}</dd><dt>DNS</dt><dd>{n.dns || '未配置'}</dd><dt>关联服务</dt><dd>{services.filter(s => s.networkId === n.id).length}</dd></dl><p className="muted">{n.description}</p><div className="heading-actions"><button className="button" disabled={busy} onClick={() => setZone({ ...n })}><Pencil size={13}/>编辑</button><button className="button" disabled={busy} onClick={() => { if (confirm('移除网络登记 ' + n.name + '？不会修改实际网络。'))
        void apply({ ...settings, networks: settings.networks.filter(x => x.id !== n.id) }); }}><Trash2 size={13}/>移除</button></div></article>)}</div>}
   <p className="form-note">这里维护网络资产登记和链接选择规则，不会修改实际 DNS、Caddy、路由器或 Kubernetes 网络。关联服务的网络区域需先解除关联再删除。</p>
  </section>
  <section className="panel table-panel"><div className="section-heading"><h2>服务内外网入口清单</h2><span className="muted">地址是否配置，不代表连接或认证已通过</span></div><div className="table-scroll"><table><thead><tr><th>服务</th><th>网络区域</th><th>内网地址</th><th>外网地址</th><th>操作</th></tr></thead><tbody>{services.map(s => <tr key={s.id}><td>{s.name}</td><td>{settings.networks.find(n => n.id === s.networkId)?.name || '未归类'}</td>{(['internal', 'external'] as const).map(scope => <td key={scope}>{serviceEndpoint(s, scope) ? <a className="endpoint-link" href={serviceEndpoint(s, scope)} target="_blank" rel="noreferrer">{serviceEndpoint(s, scope)}</a> : <span className="muted">未配置</span>}</td>)}<td><button className="text-button" onClick={() => editService(s)}>配置地址</button></td></tr>)}</tbody></table></div></section>
  {zone && <div className="overlay"><section className="dialog" role="dialog" aria-modal="true" aria-labelledby="zone-title"><div className="section-heading"><h2 id="zone-title">{zone.id ? '编辑网络区域' : '新增网络区域'}</h2><button disabled={busy} onClick={() => setZone(null)}>关闭</button></div>{error && <div className="error" role="alert">{error}</div>}<form onSubmit={saveZone}><label>网络名称<input value={zone.name} maxLength={80} required onChange={e => setZone({ ...zone, name: e.target.value })}/></label><label>网络类型<select value={zone.scope} onChange={e => setZone({ ...zone, scope: e.target.value as NetworkZone['scope'] })}><option value="internal">内网</option><option value="external">外网</option><option value="tunnel">穿透 / VPN</option></select></label><label>网段（选填，CIDR）<input value={zone.cidr} placeholder="192.168.1.0/24" onChange={e => setZone({ ...zone, cidr: e.target.value })}/></label><label>网关或入口（登记信息）<input value={zone.gateway} maxLength={256} onChange={e => setZone({ ...zone, gateway: e.target.value })}/></label><label>DNS（登记信息）<input value={zone.dns} maxLength={256} onChange={e => setZone({ ...zone, dns: e.target.value })}/></label><label>说明<textarea value={zone.description} maxLength={500} onChange={e => setZone({ ...zone, description: e.target.value })}/></label><button className="button primary" disabled={busy}>{busy ? '保存中…' : '保存网络区域'}</button></form></section></div>}
 </div>;
}
