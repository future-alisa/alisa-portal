export type AccessMode = 'auto' | 'internal' | 'external';
export type NetworkZone = {
    id: string;
    name: string;
    scope: 'internal' | 'external' | 'tunnel';
    cidr: string;
    gateway: string;
    dns: string;
    description: string;
};
export type NetworkSettings = {
    revision: number;
    internalHosts: string[];
    externalHosts: string[];
    networks: NetworkZone[];
};
export type Service = {
    id: string;
    name: string;
    category: string;
    tenant: string;
    url: string;
    internalUrl?: string;
    externalUrl?: string;
    networkId?: string;
    status: string;
    description: string;
};
export const emptyNetworkSettings: NetworkSettings = { revision: 0, internalHosts: [], externalHosts: [], networks: [] };
export function normalizeHost(host: string): string { return host.toLowerCase().replace(/^\[|\]$/g, '').replace(/\.$/, ''); }
export function isPrivateHost(value: string): boolean {
    const host = normalizeHost(value);
    if (host === 'localhost' || host.endsWith('.localhost') || host.endsWith('.local'))
        return true;
    const ipv4 = host.split('.');
    if (ipv4.length === 4 && ipv4.every(x => /^\d{1,3}$/.test(x) && Number(x) <= 255)) {
        const [a, b] = ipv4.map(Number);
        return a === 10 || a === 127 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168) || (a === 169 && b === 254);
    }
    if (host === '::1')
        return true;
    if (host.startsWith('::ffff:'))
        return isPrivateHost(host.slice(7));
    if (host.includes(':')) {
        const prefix = parseInt(host.split(':')[0], 16);
        return (prefix & 0xfe00) === 0xfc00 || (prefix & 0xffc0) === 0xfe80;
    }
    return false;
}
export function detectAccess(host: string, settings: NetworkSettings): {
    scope: 'internal' | 'external';
    reason: string;
} {
    const normalized = normalizeHost(host);
    if (settings.externalHosts.some(h => normalizeHost(h) === normalized))
        return { scope: 'external', reason: '匹配外网门户入口规则' };
    if (settings.internalHosts.some(h => normalizeHost(h) === normalized))
        return { scope: 'internal', reason: '匹配内网门户入口规则' };
    if (isPrivateHost(normalized))
        return { scope: 'internal', reason: '门户使用私有地址或本地域名' };
    return { scope: 'external', reason: '未匹配内网规则，默认使用外网入口' };
}
export function serviceEndpoint(service: Service, scope: 'internal' | 'external'): string {
    const raw = scope === 'internal' ? service.internalUrl : (service.externalUrl || service.url);
    if (!raw)
        return '';
    try {
        const u = new URL(raw);
        return ['https:', 'http:'].includes(u.protocol) && !u.username && !u.password ? raw : '';
    }
    catch {
        return '';
    }
}
