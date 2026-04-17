import { type VariantProps, cva } from 'class-variance-authority'

export const stepVariants = cva(
  'flex items-center text-sm font-medium',
  {
    variants: {
      variant: {
        completed: 'text-primary',
        current: 'text-primary',
        pending: 'text-muted-foreground'
      }
    },
    defaultVariants: {
      variant: 'pending'
    }
  }
)

export type StepVariants = VariantProps<typeof stepVariants>