import * as SelectPrimitive from '@radix-ui/react-select'
import { Check, ChevronDown } from 'lucide-react'
import type { ReactNode } from 'react'

export function Select({ value, onValueChange, placeholder, children, className = '' }: { value?: string; onValueChange: (value: string) => void; placeholder?: string; children: ReactNode; className?: string }) {
  return <SelectPrimitive.Root value={value} onValueChange={onValueChange}>
    <SelectPrimitive.Trigger className={`select-trigger ${className}`} aria-label={placeholder}>
      <SelectPrimitive.Value placeholder={placeholder} /><SelectPrimitive.Icon><ChevronDown size={15} /></SelectPrimitive.Icon>
    </SelectPrimitive.Trigger>
    <SelectPrimitive.Portal><SelectPrimitive.Content className="select-content" position="popper" sideOffset={5}>
      <SelectPrimitive.Viewport>{children}</SelectPrimitive.Viewport>
    </SelectPrimitive.Content></SelectPrimitive.Portal>
  </SelectPrimitive.Root>
}

export function SelectItem({ value, children }: { value: string; children: ReactNode }) {
  return <SelectPrimitive.Item value={value} className="select-item"><SelectPrimitive.ItemText>{children}</SelectPrimitive.ItemText><SelectPrimitive.ItemIndicator><Check size={14} /></SelectPrimitive.ItemIndicator></SelectPrimitive.Item>
}
