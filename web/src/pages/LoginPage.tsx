import { useState } from 'react'
import { login } from '../api'
import LogoMark from '../components/LogoMark'

interface Props {
  onLogin: () => void
  onBack: () => void
}

export default function LoginPage({ onLogin, onBack }: Props) {
  const [key, setKey] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await login(key)
      onLogin()
    } catch {
      setError('Invalid API key. Please try again.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      minHeight: '100vh',
      background: '#FBF0DC',
      fontFamily: "'Lato', system-ui, sans-serif",
    }}>
      <div style={{ width: 400, position: 'relative' }}>

        {/* Back link */}
        <button
          onClick={onBack}
          style={{
            background: 'none', border: 'none', cursor: 'pointer',
            fontFamily: "'Bebas Neue', Impact, sans-serif",
            fontSize: 13, letterSpacing: 2, color: '#9A7B5A',
            marginBottom: 28, display: 'block', padding: 0,
          }}
          onMouseEnter={e => (e.currentTarget.style.color = '#C23B22')}
          onMouseLeave={e => (e.currentTarget.style.color = '#9A7B5A')}
        >
          ← BACK
        </button>

        {/* Card */}
        <div style={{
          background: '#FFFDF7',
          border: '1px solid #D4B896',
          boxShadow: '5px 5px 0 rgba(92,58,30,.15)',
          padding: '40px 36px',
        }}>
          {/* Logo */}
          <div style={{ textAlign: 'center', marginBottom: 32 }}>
            <LogoMark size={36} color="#2C1A0E" />
            <div style={{
              fontFamily: "'Bebas Neue', Impact, sans-serif",
              fontSize: 22, letterSpacing: 4, color: '#2C1A0E', marginTop: 8,
            }}>
              RUNRIGHT
            </div>
          </div>

          {/* Rule */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 28 }}>
            <div style={{ flex: 1, height: 1, background: '#D4B896' }} />
            <span style={{ fontFamily: "'Bebas Neue'", fontSize: 11, letterSpacing: 3, color: '#9A7B5A' }}>
              SIGN IN
            </span>
            <div style={{ flex: 1, height: 1, background: '#D4B896' }} />
          </div>

          <form onSubmit={handleSubmit}>
            <div style={{ marginBottom: 20 }}>
              <label style={{
                display: 'block',
                fontFamily: "'Bebas Neue', Impact, sans-serif",
                fontSize: 12, letterSpacing: 1.5, color: '#6B4226',
                marginBottom: 8, textTransform: 'uppercase',
              }}>
                API Key
              </label>
              <input
                type="password"
                placeholder="Enter your RUNRIGHT_API_KEY"
                value={key}
                onChange={e => setKey(e.target.value)}
                autoFocus
                autoComplete="current-password"
                style={{
                  width: '100%',
                  background: '#FBF0DC',
                  border: '1px solid #D4B896',
                  borderBottom: '2px solid #B8946A',
                  color: '#2C1A0E',
                  padding: '11px 14px',
                  fontFamily: "'Lato', system-ui, sans-serif",
                  fontSize: 14, outline: 'none',
                }}
                onFocus={e => { e.target.style.borderColor = '#B8860B'; e.target.style.boxShadow = '0 2px 0 #B8860B' }}
                onBlur={e => { e.target.style.borderColor = '#D4B896'; e.target.style.borderBottomColor = '#B8946A'; e.target.style.boxShadow = 'none' }}
              />
            </div>

            {error && (
              <p style={{ fontSize: 13, color: '#C23B22', marginBottom: 16, lineHeight: 1.5 }}>
                {error}
              </p>
            )}

            <button
              type="submit"
              disabled={loading || key === ''}
              style={{
                width: '100%',
                background: key && !loading ? '#C23B22' : '#D4B896',
                color: '#FFFDF7',
                border: 'none',
                padding: '13px',
                fontFamily: "'Bebas Neue', Impact, sans-serif",
                fontSize: 16, letterSpacing: 2, cursor: key === '' || loading ? 'not-allowed' : 'pointer',
                boxShadow: key && !loading ? '3px 3px 0 rgba(92,58,30,.2)' : 'none',
                transition: 'all .1s',
              }}
              onMouseEnter={e => { if (key && !loading) { (e.currentTarget).style.background = '#9B2D17'; (e.currentTarget).style.transform = 'translate(1px,1px)'; (e.currentTarget).style.boxShadow = '2px 2px 0 rgba(92,58,30,.2)' } }}
              onMouseLeave={e => { (e.currentTarget).style.background = key && !loading ? '#C23B22' : '#D4B896'; (e.currentTarget).style.transform = 'none'; (e.currentTarget).style.boxShadow = key && !loading ? '3px 3px 0 rgba(92,58,30,.2)' : 'none' }}
            >
              {loading ? 'Signing in…' : 'Sign In'}
            </button>
          </form>

          <p style={{ fontSize: 12, color: '#9A7B5A', textAlign: 'center', marginTop: 20, lineHeight: 1.7 }}>
            Key exchanged once · HttpOnly cookie set<br />Never stored in browser
          </p>
        </div>
      </div>
    </div>
  )
}
