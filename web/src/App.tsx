import { useState, type CSSProperties } from 'react'
import { BrowserRouter, Routes, Route, NavLink, Link, Outlet, Navigate, useNavigate } from 'react-router-dom'
import JobsPage from './pages/JobsPage'
import JobDetailPage from './pages/JobDetailPage'
import JobGroupPage from './pages/JobGroupPage'
import CatalogPage from './pages/CatalogPage'
import SettingsPage from './pages/SettingsPage'
import LoginPage from './pages/LoginPage'
import LandingPage from './pages/LandingPage'
import InstallPage from './pages/InstallPage'
import { logout } from './api'
import LogoMark from './components/LogoMark'
import './App.css'

export default function App() {
  return (
    <BrowserRouter>
      <AppRoutes />
    </BrowserRouter>
  )
}

function AppRoutes() {
  const [authed, setAuthed] = useState(false)
  const navigate = useNavigate()

  async function handleLogout() {
    try { await logout() } catch { /* ignore */ }
    setAuthed(false)
    navigate('/')
  }

  return (
    <Routes>
      <Route path="/" element={<LandingPage onEnter={() => navigate('/login')} />} />
      <Route path="/install" element={<InstallPage />} />
      <Route
        path="/login"
        element={
          <LoginPage
            onLogin={() => { setAuthed(true); navigate('/app') }}
            onBack={() => navigate('/')}
          />
        }
      />
      <Route
        path="/app"
        element={authed ? <AppShell onLogout={handleLogout} /> : <Navigate to="/login" replace />}
      >
        <Route index element={<JobsPage />} />
        <Route path="jobs/group/:jobId" element={<JobGroupPage />} />
        <Route path="jobs/:id" element={<JobDetailPage />} />
        <Route path="catalog" element={<CatalogPage />} />
        <Route path="settings" element={<SettingsPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}

function AppShell({ onLogout }: { onLogout: () => void }) {
  const [dark, setDark] = useState(() =>
    typeof window !== 'undefined' ? localStorage.getItem('rr-theme') === 'dark' : false
  )

  function toggleDark() {
    setDark(d => {
      const next = !d
      localStorage.setItem('rr-theme', next ? 'dark' : 'light')
      return next
    })
  }

  const sidebarBtnStyle: CSSProperties = {
    background: 'none',
    border: 'none',
    color: 'rgba(251,240,220,.3)',
    cursor: 'pointer',
    textAlign: 'left',
    padding: '9px 12px',
    fontFamily: "'Bebas Neue', Impact, sans-serif",
    fontSize: 14,
    letterSpacing: 2,
    transition: 'color .15s',
    display: 'flex',
    alignItems: 'center',
    gap: 8,
  }

  return (
    <div className={`app${dark ? ' dark-mode' : ''}`}>
      <nav className="sidebar">
        <Link
          to="/app"
          className="logo"
          style={{ textDecoration: 'none' }}
          aria-label="RunRight home"
        >
          <span className="logo-icon"><LogoMark size={22} color="#FBF0DC" /></span>
          <span className="logo-text">RUNRIGHT</span>
        </Link>
        <NavLink to="/app" end className={({ isActive }) => isActive ? 'nav-link active' : 'nav-link'}>
          Jobs
        </NavLink>
        <NavLink to="/app/catalog" className={({ isActive }) => isActive ? 'nav-link active' : 'nav-link'}>
          Catalog
        </NavLink>
        <NavLink to="/app/settings" className={({ isActive }) => isActive ? 'nav-link active' : 'nav-link'}>
          Settings
        </NavLink>
        <div style={{ marginTop: 'auto', display: 'flex', flexDirection: 'column' }}>
          <button
            onClick={toggleDark}
            aria-label={dark ? 'Switch to light mode' : 'Switch to dark mode'}
            style={sidebarBtnStyle}
            onMouseEnter={e => (e.currentTarget.style.color = 'rgba(251,240,220,.7)')}
            onMouseLeave={e => (e.currentTarget.style.color = 'rgba(251,240,220,.3)')}
          >
            {dark ? (
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <circle cx="12" cy="12" r="5"/>
                <line x1="12" y1="1" x2="12" y2="3"/>
                <line x1="12" y1="21" x2="12" y2="23"/>
                <line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/>
                <line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/>
                <line x1="1" y1="12" x2="3" y2="12"/>
                <line x1="21" y1="12" x2="23" y2="12"/>
                <line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/>
                <line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/>
              </svg>
            ) : (
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>
              </svg>
            )}
            {dark ? 'Light' : 'Dark'}
          </button>
          <button
            onClick={onLogout}
            style={sidebarBtnStyle}
            onMouseEnter={e => (e.currentTarget.style.color = '#C23B22')}
            onMouseLeave={e => (e.currentTarget.style.color = 'rgba(251,240,220,.3)')}
          >
            Sign Out
          </button>
        </div>
      </nav>
      <main className="content">
        <Outlet />
      </main>
    </div>
  )
}
