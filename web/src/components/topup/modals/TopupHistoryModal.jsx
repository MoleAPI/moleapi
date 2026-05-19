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
import React, { useState, useEffect, useMemo } from 'react';
import {
  Modal,
  Table,
  Badge,
  Typography,
  Toast,
  Empty,
  Button,
  Input,
  Tag,
  Descriptions,
} from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { Coins } from 'lucide-react';
import { IconSearch } from '@douyinfe/semi-icons';
import { API, timestamp2string } from '../../../helpers';
import { isAdmin } from '../../../helpers/utils';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
const { Text } = Typography;

// 状态映射配置
const STATUS_CONFIG = {
  success: { type: 'success', key: '成功' },
  pending: { type: 'warning', key: '待支付' },
  failed: { type: 'danger', key: '失败' },
  expired: { type: 'danger', key: '已过期' },
};

// 支付方式映射
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

const TopupHistoryModal = ({ visible, onCancel, t }) => {
  const [loading, setLoading] = useState(false);
  const [topups, setTopups] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [keyword, setKeyword] = useState('');
  const [detailRecord, setDetailRecord] = useState(null);
  const isMobile = useIsMobile();

  const loadTopups = async (currentPage, currentPageSize) => {
    setLoading(true);
    try {
      const base = isAdmin() ? '/api/user/topup' : '/api/user/topup/self';
      const qs =
        `p=${currentPage}&page_size=${currentPageSize}` +
        (keyword ? `&keyword=${encodeURIComponent(keyword)}` : '');
      const endpoint = `${base}?${qs}`;
      const res = await API.get(endpoint);
      const { success, message, data } = res.data;
      if (success) {
        setTopups(data.items || []);
        setTotal(data.total || 0);
      } else {
        Toast.error({ content: message || t('加载失败') });
      }
    } catch (error) {
      Toast.error({ content: t('加载账单失败') });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (visible) {
      loadTopups(page, pageSize);
    }
  }, [visible, page, pageSize, keyword]);

  const handlePageChange = (currentPage) => {
    setPage(currentPage);
  };

  const handlePageSizeChange = (currentPageSize) => {
    setPageSize(currentPageSize);
    setPage(1);
  };

  const handleKeywordChange = (value) => {
    setKeyword(value);
    setPage(1);
  };

  // 管理员补单
  const handleAdminComplete = async (tradeNo) => {
    try {
      const res = await API.post('/api/user/topup/complete', {
        trade_no: tradeNo,
      });
      const { success, message } = res.data;
      if (success) {
        Toast.success({ content: t('补单成功') });
        await loadTopups(page, pageSize);
      } else {
        Toast.error({ content: message || t('补单失败') });
      }
    } catch (e) {
      Toast.error({ content: t('补单失败') });
    }
  };

  const confirmAdminComplete = (tradeNo) => {
    Modal.confirm({
      title: t('确认补单'),
      content: t('是否将该订单标记为成功并为用户入账？'),
      onOk: () => handleAdminComplete(tradeNo),
    });
  };

  // 渲染状态徽章
  const renderStatusBadge = (status) => {
    const config = STATUS_CONFIG[status] || { type: 'primary', key: status };
    return (
      <span className='flex items-center gap-2'>
        <Badge dot type={config.type} />
        <span>{t(config.key)}</span>
      </span>
    );
  };

  // 渲染支付方式
  const renderPaymentMethod = (pm) => {
    const displayName = PAYMENT_METHOD_MAP[pm];
    return <Text>{displayName ? t(displayName) : pm || '-'}</Text>;
  };

  const formatMoney = (money) => {
    const numeric = Number(money || 0);
    return `¥${numeric.toFixed(2)}`;
  };

  const getTopupAmountText = (record) => {
    if (isSubscriptionTopup(record)) {
      return t('订阅套餐');
    }
    return record?.amount_display !== undefined && record?.amount_display !== null
      ? String(record.amount_display)
      : String(record?.amount ?? '-');
  };

  const getStatusText = (status) => {
    const config = STATUS_CONFIG[status] || { key: status || '-' };
    return t(config.key);
  };

  const getProviderText = (provider) => {
    if (!provider) return '-';
    const displayName = PAYMENT_METHOD_MAP[provider];
    return displayName ? t(displayName) : provider;
  };

  const openDetail = (record) => {
    setDetailRecord(record);
  };

  const closeDetail = () => {
    setDetailRecord(null);
  };

  const isSubscriptionTopup = (record) => {
    const tradeNo = (record?.trade_no || '').toLowerCase();
    return Number(record?.amount || 0) === 0 && tradeNo.startsWith('sub');
  };

  // 检查是否为管理员
  const userIsAdmin = useMemo(() => isAdmin(), []);

  const columns = useMemo(() => {
    const baseColumns = [
      ...(userIsAdmin
        ? [
            {
              title: t('用户ID'),
              dataIndex: 'user_id',
              key: 'user_id',
              render: (userId) => <Text>{userId ?? '-'}</Text>,
            },
          ]
        : []),
      {
        title: t('订单号'),
        dataIndex: 'trade_no',
        key: 'trade_no',
        render: (text) => <Text copyable>{text}</Text>,
      },
      {
        title: t('支付方式'),
        dataIndex: 'payment_method',
        key: 'payment_method',
        render: renderPaymentMethod,
      },
          {
            title: t('充值额度'),
            dataIndex: 'amount',
            key: 'amount',
            render: (amount, record) => {
              if (isSubscriptionTopup(record)) {
                return (
                  <Tag color='purple' shape='circle' size='small'>
                    {t('订阅套餐')}
                  </Tag>
                );
              }
              const displayAmount =
                record?.amount_display !== undefined && record?.amount_display !== null
                  ? record.amount_display
                  : amount;
              return (
                <span className='flex items-center gap-1'>
                  <Coins size={16} />
                  <Text>{displayAmount}</Text>
                </span>
              );
            },
          },
      {
        title: t('支付金额'),
        dataIndex: 'money',
        key: 'money',
        render: (money) => <Text type='danger'>{formatMoney(money)}</Text>,
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        key: 'status',
        render: renderStatusBadge,
      },
    ];

    baseColumns.push({
      title: t('创建时间'),
      dataIndex: 'create_time',
      key: 'create_time',
      render: (time) => timestamp2string(time),
    });

    baseColumns.push({
      title: t('操作'),
      key: 'action',
      render: (_, record) => {
        const actions = [
          <Button
            key='detail'
            size='small'
            type='primary'
            theme='borderless'
            onClick={() => openDetail(record)}
          >
            {t('查看详情')}
          </Button>,
        ];
        if (userIsAdmin && record.status === 'pending') {
          actions.push(
            <Button
              key='complete'
              size='small'
              type='primary'
              theme='outline'
              onClick={() => confirmAdminComplete(record.trade_no)}
            >
              {t('补单')}
            </Button>,
          );
        }
        return <div className='flex items-center gap-2'>{actions}</div>;
      },
    });

    return baseColumns;
  }, [t, userIsAdmin]);

  const detailData = useMemo(() => {
    if (!detailRecord) return [];
    return [
      ...(userIsAdmin
        ? [
            {
              key: t('用户ID'),
              value: detailRecord.user_id ?? '-',
            },
          ]
        : []),
      {
        key: t('订单号'),
        value: <Text copyable>{detailRecord.trade_no || '-'}</Text>,
      },
      {
        key: t('上游订单号'),
        value: detailRecord.gateway_trade_no ? (
          <Text copyable>{detailRecord.gateway_trade_no}</Text>
        ) : (
          '-'
        ),
      },
      {
        key: t('支付方式'),
        value: getProviderText(detailRecord.payment_method),
      },
      {
        key: t('支付渠道'),
        value: getProviderText(detailRecord.payment_provider),
      },
      {
        key: t('充值额度'),
        value: getTopupAmountText(detailRecord),
      },
      {
        key: t('支付金额'),
        value: formatMoney(detailRecord.money),
      },
      {
        key: t('状态'),
        value: getStatusText(detailRecord.status),
      },
      {
        key: t('创建时间'),
        value: detailRecord.create_time ? timestamp2string(detailRecord.create_time) : '-',
      },
      {
        key: t('完成时间'),
        value: detailRecord.complete_time
          ? timestamp2string(detailRecord.complete_time)
          : '-',
      },
    ];
  }, [detailRecord, t, userIsAdmin]);

  return (
    <>
      <Modal
        title={t('充值账单')}
        visible={visible}
        onCancel={onCancel}
        footer={null}
        size={isMobile ? 'full-width' : 'large'}
      >
        <div className='mb-3'>
          <Input
            prefix={<IconSearch />}
            placeholder={t('订单号')}
            value={keyword}
            onChange={handleKeywordChange}
            showClear
          />
        </div>
        <Table
          columns={columns}
          dataSource={topups}
          loading={loading}
          rowKey='id'
          pagination={{
            currentPage: page,
            pageSize: pageSize,
            total: total,
            showSizeChanger: true,
            pageSizeOpts: [10, 20, 50, 100],
            onPageChange: handlePageChange,
            onPageSizeChange: handlePageSizeChange,
          }}
          size='small'
          empty={
            <Empty
              image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
              darkModeImage={
                <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
              }
              description={t('暂无充值记录')}
              style={{ padding: 30 }}
            />
          }
        />
      </Modal>
      <Modal
        title={t('充值详情')}
        visible={!!detailRecord}
        onCancel={closeDetail}
        footer={null}
        size={isMobile ? 'full-width' : 'medium'}
      >
        {detailRecord && (
          <Descriptions
            data={detailData}
            row
            size='small'
            className='topup-history-detail'
          />
        )}
      </Modal>
    </>
  );
};

export default TopupHistoryModal;
