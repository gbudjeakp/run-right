import { useState } from 'react'
import { login } from '../api'
import LogoMark from '../components/LogoMark'

interface Props {
  onLogin: () => void
}

export default function LoginPage({ onLogin }: Props) {
  const [key, setKey] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const allowEmptyInDev = import.meta.env.DEV

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await login(key || 'dev')
      onLogin()
    } catch {
      setError('Invalid API key. Please try again.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex items-center justify-center min-h-screen bg-[var(--cream)] px-4 py-8 font-sans">
      <div className="w-full max-w-sm">

        {/* Card */}
        <div className="bg-paper border border-[var(--border)] shadow-[5px_5px_0_rgba(92,58,30,.15)] px-8 py-10 sm:px-10">

          {/* Logo */}
          <div className="flex flex-col items-center mb-8">
            <LogoMark size={36} color="#2C1A0E" />
            <div className="font-deco text-[22px] tracking-[4px] text-[var(--text)] mt-2">RUNRIGHT</div>
          </div>

          {/* Divider */}
          <div className="flex items-center gap-3 mb-7">
            <div className="flex-1 h-px bg-[var(--border)]" />
            <span className="font-deco text-[11px] tracking-[3px] text-[var(--text-light)]">SIGN IN</span>
            <div className="flex-1 h-px bg-[var(--border)]" />
          </div>

          <form onSubmit={handleSubmit}>
            <div className="mb-5">
              <label className="block font-deco text-[12px] tracking-[1.5px] text-[var(--text-mid)] mb-2 uppercase">
                API Key
              </label>
              <input
                type="password"
                className="rr-input !bg-cream"
                placeholder="Enter your RUNRIGHT_API_KEY"
                value={key}
                onChange={e => setKey(e.target.value)}
                autoFocus
                autoComplete="current-password"
              />
            </div>

            {error && (
              <p className="text-[13px] text-red mb-4 leading-relaxed">{error}</p>
            )}

            <button
              type="submit"
              disabled={loading || (!allowEmptyInDev && key === '')}
              className={[
                'w-full py-3 font-deco text-[16px] tracking-[2px] border-none cursor-pointer transition-all',
                (allowEmptyInDev || key) && !loading
                  ? 'bg-red text-cream shadow-rr hover:bg-red-dark hover:translate-x-px hover:translate-y-px hover:shadow-[2px_2px_0_rgba(92,58,30,.2)]'
                  : 'bg-[var(--border-dark)] text-[var(--cream-alt)] opacity-60 cursor-not-allowed',
              ].join(' ')}
            >
              {loading ? 'Signing in…' : 'Sign In'}
            </button>
          </form>

          <p className="text-xs text-[var(--text-light)] text-center mt-5 leading-relaxed">
            Key exchanged once · HttpOnly cookie set<br />Never stored in browser
          </p>
        </div>
      </div>
    </div>
  )
}
