import { useState } from 'react';
import { ChevronRight } from 'lucide-react';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@infrapro/ui/components/collapsible';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { SidebarMenuButton, SidebarMenuItem, SidebarMenuSub, SidebarMenuSubButton, SidebarMenuSubItem, useSidebar } from '@infrapro/ui/components/sidebar';
import type { LinkRenderer, NavigationItem, NavigationSection } from '../shared/types';

const menuClass = 'h-11 gap-3 px-3 text-sidebar-foreground md:h-10';

export function NavigationMenuItem({ item, renderLink }: { item: NavigationItem | NavigationSection; renderLink: LinkRenderer }) {
  const { setOpenMobile } = useSidebar();
  if ('items' in item) {
    // Permissions can reduce a section to one destination; an extra level adds no value.
    if (!item.items.length) return null;
    if (item.items.length === 1) return <NavigationMenuItem item={item.items[0]!} renderLink={renderLink} />;
    return <NavigationSubmenu item={item} renderLink={renderLink} />;
  }
  return <SidebarMenuItem>
    <SidebarMenuButton asChild isActive={item.active} tooltip={item.title} className={menuClass}>
      {renderLink({ href: item.href, 'aria-current': item.active ? 'page' : undefined, onClick: () => setOpenMobile(false), children: <>{item.icon && <item.icon className="size-4" aria-hidden="true" />}<span>{item.title}</span></> })}
    </SidebarMenuButton>
  </SidebarMenuItem>;
}

function NavigationSubmenu({ item, renderLink }: { item: NavigationSection; renderLink: LinkRenderer }) {
  const { state, isMobile, setOpenMobile } = useSidebar();
  const activeHref = item.items.find((child) => child.active)?.href;
  const [expanded, setExpanded] = useState({ activeHref, open: !!activeHref });
  // A command-search or deep-link navigation must reveal the new active child.
  if (expanded.activeHref !== activeHref) setExpanded({ activeHref, open: activeHref ? true : expanded.open });
  const label = <>{item.icon && <item.icon className="size-4" aria-hidden="true" />}<span>{item.title}</span><ChevronRight className="ml-auto size-4 transition-transform group-data-[state=open]/navigation:rotate-90 motion-reduce:transition-none" aria-hidden="true" /></>;
  if (state === 'collapsed' && !isMobile) return <SidebarMenuItem>
    <DropdownMenu>
      <DropdownMenuTrigger asChild><SidebarMenuButton tooltip={item.title} aria-label={item.title} isActive={!!activeHref} className={menuClass}>{label}</SidebarMenuButton></DropdownMenuTrigger>
      <DropdownMenuContent side="right" align="start" sideOffset={8}>
        <DropdownMenuLabel>{item.title}</DropdownMenuLabel><DropdownMenuSeparator />
        {item.items.map((child) => <DropdownMenuItem key={child.href} asChild>{renderLink({ href: child.href, 'aria-current': child.active ? 'page' : undefined, children: <>{child.icon && <child.icon className="size-4" aria-hidden="true" />}{child.title}</> })}</DropdownMenuItem>)}
      </DropdownMenuContent>
    </DropdownMenu>
  </SidebarMenuItem>;
  return <Collapsible asChild open={expanded.open} onOpenChange={(open) => setExpanded({ activeHref, open })} className="group/navigation">
    <SidebarMenuItem>
      <CollapsibleTrigger asChild><SidebarMenuButton tooltip={item.title} className={menuClass}>{label}</SidebarMenuButton></CollapsibleTrigger>
      <CollapsibleContent>
        <SidebarMenuSub>
          {item.items.map((child) => <SidebarMenuSubItem key={child.href}>
            <SidebarMenuSubButton asChild isActive={child.active} className="h-auto min-h-11 py-2 md:min-h-9">
              {renderLink({ href: child.href, 'aria-current': child.active ? 'page' : undefined, onClick: () => setOpenMobile(false), children: <span>{child.title}</span> })}
            </SidebarMenuSubButton>
          </SidebarMenuSubItem>)}
        </SidebarMenuSub>
      </CollapsibleContent>
    </SidebarMenuItem>
  </Collapsible>;
}
