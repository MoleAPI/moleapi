/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Link } from '@tanstack/react-router'
import { Menu } from 'lucide-react'
import { useMemo } from 'react'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

import type { TopNavLink } from '../types'
import { TopNavLinkContent } from './top-nav-link-content'

type TopNavProps = React.HTMLAttributes<HTMLElement> & {
  links: TopNavLink[]
}

/**
 * 顶部导航栏组件
 * 在大屏幕显示水平导航，在小屏幕显示下拉菜单
 */
export function TopNav({ className, links, ...props }: TopNavProps) {
  // 规范化链接，确保所有可选属性都有默认值
  const normalizedLinks = useMemo(
    () =>
      links.map((link) => ({
        isActive: false,
        disabled: false,
        external: false,
        ...link,
      })),
    [links]
  )

  return (
    <>
      {/* 移动端下拉菜单 */}
      <div className='2xl:hidden'>
        <DropdownMenu modal={false}>
          <DropdownMenuTrigger
            render={<Button size='icon' variant='outline' className='size-7' />}
          >
            <Menu />
          </DropdownMenuTrigger>
          <DropdownMenuContent side='bottom' align='start'>
            {normalizedLinks.map((link) => {
              const { title, href, isActive, disabled, external } = link
              return (
                <DropdownMenuItem
                  key={`${title}-${href}`}
                  render={
                    external ? (
                      <a
                        href={href}
                        target='_blank'
                        rel='noopener noreferrer'
                        className={!isActive ? 'text-muted-foreground' : ''}
                      >
                        <TopNavLinkContent
                          link={link}
                          label={title}
                          iconClassName={link.iconClassName}
                        />
                      </a>
                    ) : (
                      <Link
                        to={href}
                        className={!isActive ? 'text-muted-foreground' : ''}
                        disabled={disabled}
                      >
                        <TopNavLinkContent
                          link={link}
                          label={title}
                          iconClassName={link.iconClassName}
                        />
                      </Link>
                    )
                  }
                />
              )
            })}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* 桌面端水平导航 */}
      <nav
        className={cn(
          'hidden flex-nowrap items-center gap-1 2xl:flex 2xl:gap-1.5',
          className
        )}
        {...props}
      >
        {normalizedLinks.map((link) => {
          const { title, href, isActive, disabled, external } = link
          return external ? (
            <a
              key={`${title}-${href}`}
              href={href}
              target='_blank'
              rel='noopener noreferrer'
              className={cn(
                'hover:bg-accent/70 hover:text-primary focus-visible:bg-accent/70 shrink-0 rounded-lg px-2.5 py-1.5 text-sm font-medium whitespace-nowrap motion-safe:transition-[color,background-color,transform] motion-safe:duration-200 motion-safe:ease-[cubic-bezier(0.25,1,0.5,1)] motion-safe:hover:-translate-y-px 2xl:px-3',
                !isActive && 'text-muted-foreground'
              )}
            >
              <TopNavLinkContent
                link={link}
                label={title}
                iconClassName={link.iconClassName}
              />
            </a>
          ) : (
            <Link
              key={`${title}-${href}`}
              to={href}
              disabled={disabled}
              className={cn(
                'hover:bg-accent/70 hover:text-primary focus-visible:bg-accent/70 shrink-0 rounded-lg px-2.5 py-1.5 text-sm font-medium whitespace-nowrap motion-safe:transition-[color,background-color,transform] motion-safe:duration-200 motion-safe:ease-[cubic-bezier(0.25,1,0.5,1)] motion-safe:hover:-translate-y-px 2xl:px-3',
                !isActive && 'text-muted-foreground'
              )}
            >
              <TopNavLinkContent
                link={link}
                label={title}
                iconClassName={link.iconClassName}
              />
            </Link>
          )
        })}
      </nav>
    </>
  )
}
