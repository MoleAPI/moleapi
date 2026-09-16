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
import { BubbleChatIcon, LinkSquare01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { QRCodeSVG } from 'qrcode.react'
import { useTranslation } from 'react-i18next'

import { IconDiscord, IconTelegram, IconWeChat } from '@/assets/brand-icons'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'

const channels = [
  { key: 'qq', label: 'QQ', icon: 'qq', qr: true, tone: 'info' },
  {
    key: 'wechat',
    label: 'WeChat',
    icon: 'wechat',
    qr: true,
    tone: 'success',
  },
  {
    key: 'telegram',
    label: 'Telegram',
    icon: 'telegram',
    qr: false,
    tone: 'chart-1',
  },
  {
    key: 'discord',
    label: 'Discord',
    icon: 'discord',
    qr: false,
    tone: 'chart-4',
  },
] as const

function ChannelIcon(props: { icon: (typeof channels)[number]['icon'] }) {
  if (props.icon === 'wechat') return <IconWeChat />
  if (props.icon === 'telegram') return <IconTelegram />
  if (props.icon === 'discord') return <IconDiscord />
  return <HugeiconsIcon icon={BubbleChatIcon} />
}

export function CommunityChannels(props: { links: Record<string, string> }) {
  const { t } = useTranslation()

  return (
    <section aria-labelledby='community-title' className='flex flex-col gap-3'>
      <div>
        <h2 id='community-title' className='text-sm font-semibold'>
          {t('Community')}
        </h2>
        <p className='text-muted-foreground mt-0.5 text-xs'>
          {t('Join a community for announcements and peer support.')}
        </p>
      </div>
      <div className='grid grid-cols-2 gap-3 lg:grid-cols-4'>
        {channels.map((channel) => {
          const href = props.links[channel.key]
          const trigger = (
            <Button
              variant='outline'
              disabled={!href}
              className='h-auto min-w-0 justify-start gap-3 p-3 text-left sm:p-4'
            >
              <IconBadge tone={channel.tone} size='lg'>
                <ChannelIcon icon={channel.icon} />
              </IconBadge>
              <span className='min-w-0'>
                <span className='block truncate font-medium'>
                  {channel.label}
                </span>
                <span className='text-muted-foreground block truncate text-xs font-normal'>
                  {href ? t('View community') : t('Coming soon')}
                </span>
              </span>
            </Button>
          )

          if (!href) return <div key={channel.key}>{trigger}</div>

          return (
            <Dialog
              key={channel.key}
              title={channel.label}
              description={
                channel.qr
                  ? t('Scan the QR code or open the community link.')
                  : t('You will be redirected to the community platform.')
              }
              trigger={trigger}
              contentClassName='sm:max-w-sm'
              bodyClassName='flex flex-col items-center gap-4'
              footer={
                <Button
                  render={<a href={href} target='_blank' rel='noreferrer' />}
                >
                  <HugeiconsIcon
                    icon={LinkSquare01Icon}
                    data-icon='inline-start'
                  />
                  {t('Open link')}
                </Button>
              }
            >
              {channel.qr && (
                <div className='bg-background rounded-lg border p-3'>
                  <QRCodeSVG
                    value={href}
                    size={196}
                    title={t('{{platform}} community QR code', {
                      platform: channel.label,
                    })}
                  />
                </div>
              )}
            </Dialog>
          )
        })}
      </div>
    </section>
  )
}
