/*
Copyright (C) 2025 QuantumNous

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

import React from 'react';
import { Modal, Typography, Input, InputNumber } from '@douyinfe/semi-ui';
import { CreditCard } from 'lucide-react';
import { getCurrencyConfig } from '../../../helpers';
import {
  displayAmountToQuota,
  quotaToDisplayAmount,
} from '../../../helpers/quota';

const TransferModal = ({
  t,
  openTransfer,
  transfer,
  handleTransferCancel,
  userState,
  renderQuota,
  getQuotaPerUnit,
  transferAmount,
  setTransferAmount,
}) => {
  const minimumQuota = getQuotaPerUnit();
  const availableQuota = userState?.user?.aff_quota || 0;
  const { symbol, type } = getCurrencyConfig();
  const minimumAmount = quotaToDisplayAmount(minimumQuota);
  const availableAmount = quotaToDisplayAmount(availableQuota);
  const displayTransferAmount = quotaToDisplayAmount(transferAmount);
  const inputPrecision = type === 'TOKENS' ? 0 : 6;

  return (
    <Modal
      title={
        <div className='flex items-center'>
          <CreditCard className='mr-2' size={18} />
          {t('划转邀请额度')}
        </div>
      }
      visible={openTransfer}
      onOk={transfer}
      onCancel={handleTransferCancel}
      maskClosable={false}
      centered
    >
      <div className='space-y-4'>
        <div>
          <Typography.Text strong className='block mb-2'>
            {t('可用邀请额度')}
          </Typography.Text>
          <Input
            value={renderQuota(userState?.user?.aff_quota)}
            disabled
            className='!rounded-lg'
          />
        </div>
        <div>
          <Typography.Text strong className='block mb-2'>
            {t('划转额度')} · {t('最低') + renderQuota(minimumQuota)}
          </Typography.Text>
          <InputNumber
            min={minimumAmount}
            max={availableAmount}
            value={Number(displayTransferAmount.toFixed(inputPrecision))}
            onChange={(value) => {
              const amount = value === '' || value == null ? 0 : value;
              setTransferAmount(displayAmountToQuota(amount));
            }}
            prefix={type === 'TOKENS' ? undefined : symbol}
            precision={inputPrecision}
            step={type === 'TOKENS' ? 1 : 0.01}
            className='w-full !rounded-lg'
          />
        </div>
      </div>
    </Modal>
  );
};

export default TransferModal;
