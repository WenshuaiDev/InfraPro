import type { ComponentProps } from 'react';
import { Button as BaseButton } from '@base-ui/react/button';
import { ChevronLeft, ChevronRight, MoreHorizontal } from 'lucide-react';
import { cn } from '../lib/utils';
import { buttonVariants } from './button';

export function Pagination({ className, ...props }: ComponentProps<'nav'>) {
  return <nav aria-label="分页" data-slot="pagination" className={cn('flex justify-center', className)} {...props} />;
}

export function PaginationContent({ className, ...props }: ComponentProps<'ul'>) {
  return <ul data-slot="pagination-content" className={cn('flex flex-wrap items-center justify-center gap-0.5', className)} {...props} />;
}

export function PaginationItem(props: ComponentProps<'li'>) {
  return <li data-slot="pagination-item" {...props} />;
}

type LinkProps = ComponentProps<typeof BaseButton> & { isActive?: boolean; href?: string };

export function PaginationLink({ isActive, href, className, ...props }: LinkProps) {
  return <BaseButton
    type={href ? undefined : 'button'}
    nativeButton={!href}
    render={href ? <a href={href} /> : undefined}
    aria-current={isActive ? 'page' : undefined}
    data-slot="pagination-link"
    data-active={isActive}
    className={cn(buttonVariants({ variant: isActive ? 'outline' : 'ghost', size: 'icon' }), 'min-w-9 w-auto px-2 tabular-nums max-sm:min-h-11', className)}
    {...props}
  />;
}

export function PaginationPrevious({ className, text = '上一页', ...props }: LinkProps & { text?: string }) {
  return <PaginationLink aria-label={text} className={cn('gap-1', className)} {...props}>
    <ChevronLeft aria-hidden="true" /><span className="hidden @min-[30rem]/pager:inline">{text}</span>
  </PaginationLink>;
}

export function PaginationNext({ className, text = '下一页', ...props }: LinkProps & { text?: string }) {
  return <PaginationLink aria-label={text} className={cn('gap-1', className)} {...props}>
    <span className="hidden @min-[30rem]/pager:inline">{text}</span><ChevronRight aria-hidden="true" />
  </PaginationLink>;
}

export function PaginationEllipsis() {
  return <span aria-hidden="true" data-slot="pagination-ellipsis" className="flex size-7 items-center justify-center"><MoreHorizontal className="size-4" /></span>;
}
