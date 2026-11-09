'use client';
import { useEffect, useRef, useState } from 'react';
import { ArrowRight, KeyRound, Layers3, LoaderCircle, ShieldCheck, X } from 'lucide-react';

type AuthConfig = { enabled: boolean; allowApiToken: boolean } | null;
export function LoginDialog({ config, error, close, connect }: {
    config: AuthConfig; error: string; close: () => void; connect: (token: string) => void;
}) {
    const dialog = useRef<HTMLDialogElement>(null);
    const [token, setToken] = useState('');
    useEffect(() => {
        const element = dialog.current;
        element?.showModal();
        return () => element?.close();
    }, []);
    return <dialog ref={dialog} className="login-dialog" aria-labelledby="login-title" aria-describedby="login-description" onCancel={close}>
        <button className="login-close" aria-label="关闭登录" onClick={close}><X size={19}/></button>
        <div className="login-brand"><span className="brand-icon"><Layers3 size={24}/></span><span>alisa<span className="brand-dot">.</span></span></div>
        <div className="login-heading"><span className="login-eyebrow">欢迎回来</span><h2 id="login-title">登录你的工作空间</h2><p id="login-description">一个 Alisa 账号，连接你的应用与服务。</p></div>
        {error && <div className="error" role="alert">{error}</div>}
        {config?.enabled && <>
            <div className="login-account"><span className="login-account-icon"><ShieldCheck size={22}/></span><div><strong>Alisa 统一账号</strong><small>使用已有账号安全登录</small></div><span className="login-account-tag">统一登录</span></div>
            <button className="button primary login-submit" autoFocus onClick={() => window.location.assign('/api/auth/login')}>使用 Alisa 账号继续 <ArrowRight size={17}/></button>
            <p className="login-caption">将前往 Alisa 账号中心完成登录与授权</p>
        </>}
        {!config && <div className="login-loading" role="status"><LoaderCircle size={19} className="spin"/>正在连接登录服务…</div>}
        {config && !config.enabled && <p className="form-note">统一账号登录暂未启用，请使用管理员令牌连接。</p>}
        {config?.allowApiToken && <details className="login-fallback" open={!config.enabled}><summary><KeyRound size={14}/>使用管理员令牌</summary><form onSubmit={event => { event.preventDefault(); if (token.trim()) connect(token.trim()); }}><label>管理员令牌<input type="password" autoComplete="off" required value={token} onChange={event => setToken(event.target.value)} placeholder="输入管理员访问令牌"/></label><button className="button login-submit">连接管理台 <ArrowRight size={16}/></button></form></details>}
        <div className="login-footer"><span>只想看看应用？</span><button onClick={close}>继续访客浏览</button></div>
        <p className="login-permission">管理功能需由管理员授予权限</p>
    </dialog>;
}
