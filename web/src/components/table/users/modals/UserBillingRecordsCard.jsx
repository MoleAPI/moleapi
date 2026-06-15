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

import React, { useEffect, useMemo, useState } from 'react';
import {
  Badge,
  Button,
  Empty,
  Table,
  TabPane,
  Tabs,
  Toast,
  Typography,
} from '@douyinfe/semi-ui';
import { API, renderQuota, timestamp2string } from '../../../../helpers';

const { Text } = Typography;

const STATUS_CONFIG = {
  success: { type: 'success', key: '成功' },
  pending: { type: 'warning', key: '待支付' },
  failed: { type: 'danger', key: '失败' },
  expired: { type: 'danger', key: '已过期' },
};

const LOG_TYPE_CONFIG = {
  1: { type: 'success', key: '充值' },
  3: { type: 'warning', key: '管理' },
  4: { type: 'primary', key: '系统' },
  6: { type: 'danger', key: '退款' },
};

const PAYMENT_METHOD_MAP = {
  stripe: 'Stripe',
  creem: 'Creem',
  waffo: 'Waffo',
  waffo_pancake: 'Waffo Pancake',
  lantu: '蓝兔支付',
  epay: '易支付',
  alipay: '支付宝',
  wxpay: '微信',
};

const renderBadge = (config, fallback, t) => (
  <span className='flex items-center gap-2 whitespace-nowrap'>
    <Badge dot type={config?.type || 'primary'} />
    <span>{t(config?.key || fallback || '-')}</span>
  </span>
);

const UserBillingRecordsCard = ({ userId, t, onTopupChanged }) => {
  const [activeTab, setActiveTab] = useState('topups');
  const [topups, setTopups] = useState([]);
  const [quotaLogs, setQuotaLogs] = useState([]);
  const [topupTotal, setTopupTotal] = useState(0);
  const [logTotal, setLogTotal] = useState(0);
  const [loadingTopups, setLoadingTopups] = useState(false);
  const [loadingLogs, setLoadingLogs] = useState(false);

  const loadTopups = async () => {
    if (!userId) return;
    setLoadingTopups(true);
    try {
      const res = await API.get(`/api/user/${userId}/topups?p=1&page_size=5`);
      const { success, message, data } = res.data;
      if (success) {
        setTopups(data.items || []);
        setTopupTotal(data.total || 0);
      } else {
        Toast.error({ content: message || t('加载失败') });
      }
    } catch (e) {
      Toast.error({ content: t('加载账单失败') });
    } finally {
      setLoadingTopups(false);
    }
  };

  const loadQuotaLogs = async () => {
    if (!userId) return;
    setLoadingLogs(true);
    try {
      const res = await API.get(
        `/api/user/${userId}/quota_changes?p=1&page_size=5`,
      );
      const { success, message, data } = res.data;
      if (success) {
        setQuotaLogs(data.items || []);
        setLogTotal(data.total || 0);
      } else {
        Toast.error({ content: message || t('加载失败') });
      }
    } catch (e) {
      Toast.error({ content: t('加载账单失败') });
    } finally {
      setLoadingLogs(false);
    }
  };

  useEffect(() => {
    loadTopups();
    loadQuotaLogs();
  }, [userId]);

  const completeTopup = async (tradeNo) => {
    try {
      const res = await API.post('/api/user/topup/complete', {
        trade_no: tradeNo,
      });
      const { success, message } = res.data;
      if (success) {
        Toast.success({ content: t('补单成功') });
        await loadTopups();
        await loadQuotaLogs();
        onTopupChanged?.();
      } else {
        Toast.error({ content: message || t('补单失败') });
      }
    } catch (e) {
      Toast.error({ content: t('补单失败') });
    }
  };

  const topupColumns = useMemo(
    () => [
      {
        title: t('时间'),
        dataIndex: 'create_time',
        width: 138,
        render: (time) => (
          <Text className='whitespace-nowrap'>{timestamp2string(time)}</Text>
        ),
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        width: 86,
        render: (status) => renderBadge(STATUS_CONFIG[status], status, t),
      },
      {
        title: t('金额'),
        dataIndex: 'amount',
        width: 84,
        render: (amount, record) => (
          <Text>
            {record?.amount_display !== undefined &&
            record?.amount_display !== null
              ? record.amount_display
              : amount}
          </Text>
        ),
      },
      {
        title: t('支付方式'),
        dataIndex: 'payment_method',
        width: 92,
        render: (method) => (
          <Text>{t(PAYMENT_METHOD_MAP[method] || method || '-')}</Text>
        ),
      },
      {
        title: t('订单号'),
        dataIndex: 'trade_no',
        render: (tradeNo) => (
          <Text
            copyable
            ellipsis={{ showTooltip: true }}
            style={{ maxWidth: 172 }}
          >
            {tradeNo}
          </Text>
        ),
      },
      {
        title: '',
        width: 74,
        render: (_, record) =>
          record.status === 'pending' ? (
            <Button
              size='small'
              theme='outline'
              onClick={() => completeTopup(record.trade_no)}
            >
              {t('补单')}
            </Button>
          ) : null,
      },
    ],
    [t],
  );

  const logColumns = useMemo(
    () => [
      {
        title: t('时间'),
        dataIndex: 'created_at',
        width: 138,
        render: (time) => (
          <Text className='whitespace-nowrap'>{timestamp2string(time)}</Text>
        ),
      },
      {
        title: t('类型'),
        dataIndex: 'type',
        width: 82,
        render: (type) => renderBadge(LOG_TYPE_CONFIG[type], String(type), t),
      },
      {
        title: t('额度'),
        dataIndex: 'quota',
        width: 96,
        render: (quota) => (quota ? renderQuota(quota) : '-'),
      },
      {
        title: t('内容'),
        dataIndex: 'content',
        render: (content) => (
          <Text ellipsis={{ showTooltip: true }} style={{ maxWidth: 260 }}>
            {content}
          </Text>
        ),
      },
    ],
    [t],
  );

  return (
    <Tabs
      type='line'
      size='small'
      activeKey={activeTab}
      onChange={setActiveTab}
      tabBarExtraContent={
        <Button
          size='small'
          theme='borderless'
          loading={activeTab === 'topups' ? loadingTopups : loadingLogs}
          onClick={activeTab === 'topups' ? loadTopups : loadQuotaLogs}
        >
          {t('刷新')}
        </Button>
      }
    >
      <TabPane tab={`${t('充值记录')} (${topupTotal})`} itemKey='topups'>
        <Table
          columns={topupColumns}
          dataSource={topups}
          rowKey='id'
          size='small'
          loading={loadingTopups}
          pagination={false}
          scroll={{ x: 666 }}
          empty={<Empty description={t('暂无充值记录')} />}
        />
      </TabPane>
      <TabPane tab={`${t('额度变动')} (${logTotal})`} itemKey='quota'>
        <Table
          columns={logColumns}
          dataSource={quotaLogs}
          rowKey='id'
          size='small'
          loading={loadingLogs}
          pagination={false}
          scroll={{ x: 512 }}
          empty={<Empty description={t('暂无额度变动记录')} />}
        />
      </TabPane>
    </Tabs>
  );
};

export default UserBillingRecordsCard;
