import * as DropdownMenuPrimitive from '@radix-ui/react-dropdown-menu'
import type { ReactNode } from 'react'

/** 与 select.tsx 同风格的 Radix DropdownMenu 封装，样式沿用 .dropdown-content / .dropdown-item。 */
export function Menu({ open, onOpenChange, trigger, children, align = 'end' }: { open?: boolean; onOpenChange?: (open: boolean) => void; trigger: ReactNode; children: ReactNode; align?: 'start' | 'center' | 'end' }) {
  return <DropdownMenuPrimitive.Root open={open} onOpenChange={onOpenChange}>
    <DropdownMenuPrimitive.Trigger asChild>{trigger}</DropdownMenuPrimitive.Trigger>
    <DropdownMenuPrimitive.Portal>
      <DropdownMenuPrimitive.Content className="dropdown-content" align={align} sideOffset={6} collisionPadding={12}>{children}</DropdownMenuPrimitive.Content>
    </DropdownMenuPrimitive.Portal>
  </DropdownMenuPrimitive.Root>
}

export function MenuLabel({ children }: { children: ReactNode }) {
  return <DropdownMenuPrimitive.Label className="dropdown-label">{children}</DropdownMenuPrimitive.Label>
}

export function MenuSeparator() {
  return <DropdownMenuPrimitive.Separator className="dropdown-separator" />
}

export function MenuItem({ children, onSelect, danger = false }: { children: ReactNode; onSelect: () => void; danger?: boolean }) {
  return <DropdownMenuPrimitive.Item className={'dropdown-item' + (danger ? ' dropdown-item-danger' : '')} onSelect={onSelect}>{children}</DropdownMenuPrimitive.Item>
}
