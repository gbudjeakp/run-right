import { Link } from 'react-router-dom'
import LogoMark from './LogoMark'
import './PageNav.css'

interface Props {
  dark: boolean
  onToggleDark: () => void
  /** Called when Sign In is clicked. If omitted, renders a Link to /login. */
  onSignIn?: () => void
}

function GitHubIcon() {
  return (
    <svg width={16} height={16} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M12 2C6.477 2 2 6.477 2 12c0 4.418 2.865 8.166 6.839 9.489.5.092.682-.217.682-.482 0-.237-.009-.868-.013-1.703-2.782.604-3.369-1.342-3.369-1.342-.454-1.155-1.11-1.462-1.11-1.462-.908-.62.069-.608.069-.608 1.003.07 1.531 1.03 1.531 1.03.892 1.529 2.341 1.087 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.11-4.555-4.943 0-1.091.39-1.984 1.029-2.683-.103-.253-.446-1.27.098-2.647 0 0 .84-.269 2.75 1.025A9.578 9.578 0 0 1 12 6.836a9.59 9.59 0 0 1 2.504.337c1.909-1.294 2.747-1.025 2.747-1.025.546 1.377.203 2.394.1 2.647.64.699 1.028 1.592 1.028 2.683 0 3.842-2.339 4.687-4.566 4.935.359.309.678.919.678 1.852 0 1.336-.012 2.415-.012 2.743 0 .267.18.578.688.48C19.138 20.163 22 16.418 22 12c0-5.523-4.477-10-10-10Z" />
    </svg>
  )
}

function MoonIcon() {
  return (
    <svg width={15} height={15} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
    </svg>
  )
}

function SunIcon() {
  return (
    <svg width={15} height={15} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="5" />
      <line x1="12" y1="1" x2="12" y2="3" /><line x1="12" y1="21" x2="12" y2="23" />
      <line x1="4.22" y1="4.22" x2="5.64" y2="5.64" /><line x1="18.36" y1="18.36" x2="19.78" y2="19.78" />
      <line x1="1" y1="12" x2="3" y2="12" /><line x1="21" y1="12" x2="23" y2="12" />
      <line x1="4.22" y1="19.78" x2="5.64" y2="18.36" /><line x1="18.36" y1="5.64" x2="19.78" y2="4.22" />
    </svg>
  )
}

export default function PageNav({ dark, onToggleDark, onSignIn }: Props) {
  return (
    <nav className="pnav">
      <Link to="/" className="pnav-logo">
        <LogoMark size={18} color={dark ? '#FBF0DC' : '#2C1A0E'} />
        RUNRIGHT
      </Link>

      <div className="pnav-right">
        <Link to="/compare" className="pnav-link pnav-hide-mobile">Compare</Link>
        <Link to="/pricing" className="pnav-link pnav-hide-mobile">Pricing</Link>
        <Link to="/install" className="pnav-link pnav-hide-mobile">Install</Link>
        <a
          href="https://github.com/gbudjeakp/run-right"
          target="_blank"
          rel="noopener noreferrer"
          className="pnav-link pnav-hide-mobile pnav-github"
          aria-label="GitHub repository"
        >
          <GitHubIcon />
        </a>

        {onSignIn ? (
          <button className="pnav-signin" onClick={onSignIn}>Sign In</button>
        ) : (
          <Link to="/login" className="pnav-signin">Sign In</Link>
        )}

        <button
          className="pnav-theme"
          onClick={onToggleDark}
          aria-label={dark ? 'Switch to light mode' : 'Switch to dark mode'}
        >
          {dark ? <SunIcon /> : <MoonIcon />}
        </button>
      </div>
    </nav>
  )
}
