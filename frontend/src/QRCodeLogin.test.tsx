import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { QRLoginApprovalPage } from './QRCodeLogin'
import { loginURLFromQR, QRScannerDialog } from './QRCodeScanner'

afterEach(() => {
  vi.unstubAllGlobals()
  window.history.replaceState({}, '', '/')
})

describe('QRLoginApprovalPage', () => {
  it('lets an authenticated phone confirm a computer login', async () => {
    const token = 'phone-scan-token'
    window.history.replaceState({}, '', `/qr-login?token=${token}`)
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/auth') return new Response(JSON.stringify({ initialized: true, authenticated: true, username: 'teacher' }), { status: 200 })
      if (path === `/api/auth/qr/challenge?token=${token}`) return new Response(JSON.stringify({ deviceName: 'Chrome · macOS', status: 'scanned', expiresAt: 2_000_000_000 }), { status: 200 })
      if (path === '/api/public/settings') return new Response(JSON.stringify({ title: '智创课堂', subtitle: '小学信息科技课' }), { status: 200 })
      if (path === '/api/auth/qr/decision' && init?.method === 'POST') {
        expect(JSON.parse(String(init.body))).toEqual({ token, approve: true })
        return new Response(JSON.stringify({ deviceName: 'Chrome · macOS', status: 'approved', expiresAt: 2_000_000_000 }), { status: 200 })
      }
      if (path === '/api/health') return new Response(JSON.stringify({ version: 'dev', commit: 'unknown' }), { status: 200 })
      throw new Error(`unexpected request: ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<QRLoginApprovalPage />)

    expect(await screen.findByText('Chrome · macOS')).toBeInTheDocument()
    expect(screen.getByText('账号：teacher')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '确认登录' }))
    expect(await screen.findByRole('heading', { name: '已确认登录' })).toBeInTheDocument()
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/auth/qr/decision', expect.objectContaining({ method: 'POST' })))
  })
})

describe('loginURLFromQR', () => {
  it('only accepts a same-origin ClassOrbit confirmation URL', () => {
    expect(loginURLFromQR(`${window.location.origin}/qr-login?token=scan-token`)).toBe('/qr-login?token=scan-token')
    expect(loginURLFromQR('/qr-login?token=scan-token')).toBe('/qr-login?token=scan-token')
    expect(loginURLFromQR('https://example.com/qr-login?token=scan-token')).toBeNull()
    expect(loginURLFromQR(`${window.location.origin}/navigation`)).toBeNull()
  })
})

describe('QRScannerDialog', () => {
  it('provides a camera capture fallback and can be closed', async () => {
    const onClose = vi.fn()
    render(<QRScannerDialog onClose={onClose} />)
    expect(await screen.findByRole('dialog', { name: '扫描电脑登录二维码' })).toBeInTheDocument()
    expect(screen.getByLabelText('拍照识别二维码')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '关闭扫一扫' }))
    expect(onClose).toHaveBeenCalledOnce()
  })
})
