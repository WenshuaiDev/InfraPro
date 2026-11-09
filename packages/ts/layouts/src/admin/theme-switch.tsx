import { Monitor, Moon, Sun } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { useTheme } from '@infrapro/ui/theme';

export function ThemeSwitch() {
  const { theme, setTheme } = useTheme();
  return <DropdownMenu><DropdownMenuTrigger asChild><Button variant="ghost" size="icon" aria-label="切换主题"><Sun className="size-5 dark:hidden" /><Moon className="hidden size-5 dark:block" /></Button></DropdownMenuTrigger>
    <DropdownMenuContent align="end"><DropdownMenuRadioGroup value={theme} onValueChange={(value) => { if (value === 'light' || value === 'dark' || value === 'system') setTheme(value); }}>
      <DropdownMenuRadioItem value="light"><Sun className="me-2 size-4" />浅色</DropdownMenuRadioItem><DropdownMenuRadioItem value="dark"><Moon className="me-2 size-4" />深色</DropdownMenuRadioItem><DropdownMenuRadioItem value="system"><Monitor className="me-2 size-4" />跟随系统</DropdownMenuRadioItem>
    </DropdownMenuRadioGroup></DropdownMenuContent>
  </DropdownMenu>;
}
