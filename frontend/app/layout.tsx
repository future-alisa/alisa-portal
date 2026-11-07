import type { Metadata } from 'next';
import './globals.css';
export const metadata: Metadata = { title: 'Alisa · 云资源门户', description: '统一服务目录、租户与部署管理平台' };
export default function Layout({ children }: Readonly<{
    children: React.ReactNode;
}>) { return <html lang="zh-CN"><body>{children}</body></html>; }
