import type { ReactNode } from 'react';

type AppFrameProps = {
  variant: 'console' | 'screen';
  title: string;
  description: string;
  otherAppUrl: string;
  children: ReactNode;
};

export function AppFrame({ variant, title, description, otherAppUrl, children }: AppFrameProps) {
  const isScreen = variant === 'screen';
  return (
    <div className={`workspace theme-${variant}`}>
      <a className="skip-link" href="#main">跳到主要内容</a>
      <header className="topbar">
        <a className="brand" href="/" aria-label="InfraPro 首页">
          <svg className="brand-mark" viewBox="0 0 32 32" fill="none" aria-hidden="true">
            <path d="M5 26 12 6h8l7 20M8 18h16M16 6v20" stroke="currentColor" strokeWidth="2.5" strokeLinejoin="round" />
          </svg>
          <span>InfraPro</span><span className="brand-divider">/</span><span className="app-label">{isScreen ? 'Screen' : 'Console'}</span>
        </a>
        <nav aria-label="应用导航">
          <a className="app-link" href={otherAppUrl}>{isScreen ? '打开管理端' : '打开展示端'}<span aria-hidden="true">↗</span></a>
        </nav>
      </header>
      <main id="main" className="main-content">
        <div className="page-heading">
          <p className="eyebrow">{isScreen ? 'SYSTEM OVERVIEW' : 'WORKSPACE OVERVIEW'}</p>
          <h1>{title}</h1>
          <p className="page-description">{description}</p>
        </div>
        {children}
      </main>
      <footer className="footer"><span>InfraPro · 工程初始化</span><span>v0.1.0</span></footer>
    </div>
  );
}
