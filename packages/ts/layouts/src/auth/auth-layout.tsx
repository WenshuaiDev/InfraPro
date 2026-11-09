import type { ReactNode } from 'react';
import { Card, CardContent, CardFooter, CardHeader } from '@infrapro/ui/components/card';
import type { Brand } from '../shared/types';

export function AuthLayout({ brand, children }: { brand: Brand; children: ReactNode }) {
  return <main id="main" className="flex min-h-svh w-full items-center justify-center px-8 py-8">
    <div className="mx-auto flex w-full max-w-sm flex-col justify-center space-y-2 sm:p-0">
      <div className="mb-4 flex items-center justify-center gap-2"><span className="size-7 [&>svg]:size-full" aria-hidden="true">{brand.logo}</span><h1 className="text-xl font-medium">{brand.name}</h1></div>
      {children}
    </div>
  </main>;
}

export function AuthCard({ title, description, children, footer }: { title: string; description?: ReactNode; children?: ReactNode; footer?: ReactNode }) {
  return <Card className="w-full max-w-sm gap-4"><CardHeader><h2 className="text-lg font-semibold tracking-tight">{title}</h2>{description && <div className="text-sm text-muted-foreground">{description}</div>}</CardHeader>
    {children && <CardContent>{children}</CardContent>}{footer && <CardFooter className="text-sm text-muted-foreground">{footer}</CardFooter>}
  </Card>;
}
