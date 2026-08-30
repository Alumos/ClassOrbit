import * as DialogPrimitive from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from './ui'

export function Dialog({ open, onOpenChange, title, description, children, footer, width = 'normal' }: { open: boolean; onOpenChange: (open: boolean) => void; title: string; description?: string; children: ReactNode; footer?: ReactNode; width?: 'normal' | 'wide' | 'schedule' }) {
  return <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
    <DialogPrimitive.Portal><DialogPrimitive.Overlay className="dialog-overlay" /><DialogPrimitive.Content className={`dialog-content dialog-${width}`}>
      <div className="dialog-header"><div><DialogPrimitive.Title>{title}</DialogPrimitive.Title>{description && <DialogPrimitive.Description>{description}</DialogPrimitive.Description>}</div><DialogPrimitive.Close asChild><Button variant="ghost" size="icon" aria-label="关闭"><X size={17} /></Button></DialogPrimitive.Close></div>
      <div className="dialog-body">{children}</div>{footer && <div className="dialog-footer">{footer}</div>}
    </DialogPrimitive.Content></DialogPrimitive.Portal>
  </DialogPrimitive.Root>
}
