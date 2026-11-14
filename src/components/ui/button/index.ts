import type { VariantProps } from 'class-variance-authority'
import { cva } from 'class-variance-authority'

export { default as Button } from './Button.vue'

// The buttons of the mockups: plain, primary on the brand gradient, ghost, danger.
export const buttonVariants = cva(
  "inline-flex shrink-0 cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-lg border text-[13px] leading-none font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-45 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        default: 'border-line2 bg-s3 text-t1 hover:bg-s4',
        primary: 'border-transparent bg-[image:var(--grad)] font-semibold text-white hover:brightness-110',
        ghost: 'border-transparent bg-transparent text-t2 hover:bg-s4 hover:text-t1',
        danger: 'border-bad/40 bg-transparent text-bad hover:bg-bad/10',
      },
      size: {
        default: 'h-9 px-3.5',
        sm: 'h-[30px] gap-1.5 px-2.5 text-[12.5px]',
        lg: 'h-[46px] px-4 text-sm',
        start: 'h-[52px] rounded-[10px] px-4 text-[15px]',
        icon: 'size-[34px] p-0',
        'icon-sm': 'size-[30px] p-0',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
)
export type ButtonVariants = VariantProps<typeof buttonVariants>
