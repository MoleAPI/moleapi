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
  DatePicker,
} from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { Coins, Download, FileText } from 'lucide-react';
import { IconSearch } from '@douyinfe/semi-icons';
import { API, timestamp2string } from '../../../helpers';
import { isAdmin } from '../../../helpers/utils';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
const { Text } = Typography;
const THIRTY_DAYS_SECONDS = 30 * 24 * 60 * 60;

const getDefaultUserDateRange = () => {
  const now = Math.floor(Date.now() / 1000);
  return [
    timestamp2string(now - THIRTY_DAYS_SECONDS),
    timestamp2string(now + 3600),
  ];
};

const toTimestamp = (value) => {
  if (!value) return 0;
  const timestamp = Date.parse(value);
  return Number.isNaN(timestamp) ? 0 : Math.floor(timestamp / 1000);
};

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
  const [userKeyword, setUserKeyword] = useState('');
  const [dateRange, setDateRange] = useState(() =>
    isAdmin() ? [] : getDefaultUserDateRange(),
  );
  const [detailRecord, setDetailRecord] = useState(null);
  const isMobile = useIsMobile();
  const userIsAdmin = useMemo(() => isAdmin(), []);

  const loadTopups = async (currentPage, currentPageSize) => {
    setLoading(true);
    try {
      const base = userIsAdmin ? '/api/user/topup' : '/api/user/topup/self';
      const params = new URLSearchParams({
        p: String(currentPage),
        page_size: String(currentPageSize),
      });
      if (keyword) {
        params.set('keyword', keyword);
      }
      if (userIsAdmin && userKeyword) {
        params.set('user_keyword', userKeyword);
      }
      if (Array.isArray(dateRange) && dateRange.length === 2) {
        const startTimestamp = toTimestamp(dateRange[0]);
        const endTimestamp = toTimestamp(dateRange[1]);
        if (startTimestamp > 0) {
          params.set('start_timestamp', String(startTimestamp));
        }
        if (endTimestamp > 0) {
          params.set('end_timestamp', String(endTimestamp));
        }
      }
      const endpoint = `${base}?${params.toString()}`;
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
  }, [visible, page, pageSize, keyword, userKeyword, dateRange]);

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

  const handleUserKeywordChange = (value) => {
    setUserKeyword(value);
    setPage(1);
  };

  const handleDateRangeChange = (value) => {
    setDateRange(value || (userIsAdmin ? [] : getDefaultUserDateRange()));
    setPage(1);
  };

  const resetFilters = () => {
    setKeyword('');
    setUserKeyword('');
    setDateRange(userIsAdmin ? [] : getDefaultUserDateRange());
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
      <span className='flex items-center gap-2 whitespace-nowrap'>
        <Badge dot type={config.type} />
        <span>{t(config.key)}</span>
      </span>
    );
  };

  // 渲染支付方式
  const renderPaymentMethod = (pm) => {
    const displayName = PAYMENT_METHOD_MAP[pm];
    return (
      <Text
        ellipsis={{ showTooltip: true }}
        style={{ display: 'inline-block', maxWidth: 104, whiteSpace: 'nowrap' }}
      >
        {displayName ? t(displayName) : pm || '-'}
      </Text>
    );
  };

  const formatMoney = (money) => {
    const numeric = Number(money || 0);
    return `¥${numeric.toFixed(2)}`;
  };

  const getTopupAmountText = (record) => {
    if (isSubscriptionTopup(record)) {
      return t('订阅套餐');
    }
    return record?.amount_display !== undefined &&
      record?.amount_display !== null
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

  const buildInvoiceUrl = (record, download = false) => {
    const query = download ? '?download=1' : '';
    return `/api/user/topup/${record.id}/invoice${query}`;
  };

  const getInvoiceFilename = (record) => {
    const safeTradeNo = String(record?.trade_no || 'topup').replace(
      /[^a-zA-Z0-9._-]/g,
      '_',
    );
    return `invoice-${safeTradeNo}.html`;
  };

  const fetchInvoiceHtml = async (record, download = false) => {
    const res = await API.get(buildInvoiceUrl(record, download), {
      responseType: 'blob',
      disableDuplicate: true,
      skipErrorHandler: true,
    });
    const contentType = res.headers?.['content-type'] || '';
    if (contentType.includes('application/json')) {
      const text = await res.data.text();
      let message = t('生成 Invoice 失败');
      try {
        const payload = JSON.parse(text);
        message = payload.message || message;
      } catch (e) {}
      throw new Error(message);
    }
    return res.data;
  };

  const handleViewInvoice = async (record) => {
    const popup = window.open('', '_blank');
    try {
      const blob = await fetchInvoiceHtml(record, false);
      const url = URL.createObjectURL(
        new Blob([blob], { type: 'text/html;charset=utf-8' }),
      );
      if (popup) {
        popup.location.href = url;
      } else {
        window.open(url, '_blank', 'noopener,noreferrer');
      }
      setTimeout(() => URL.revokeObjectURL(url), 5 * 60 * 1000);
    } catch (error) {
      if (popup) {
        popup.close();
      }
      Toast.error({ content: error.message || t('生成 Invoice 失败') });
    }
  };

  const handleDownloadInvoice = async (record) => {
    try {
      const blob = await fetchInvoiceHtml(record, true);
      const url = URL.createObjectURL(
        new Blob([blob], { type: 'text/html;charset=utf-8' }),
      );
      const link = document.createElement('a');
      link.href = url;
      link.download = getInvoiceFilename(record);
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      setTimeout(() => URL.revokeObjectURL(url), 60 * 1000);
    } catch (error) {
      Toast.error({ content: error.message || t('下载 Invoice 失败') });
    }
  };

  const isSubscriptionTopup = (record) => {
    const tradeNo = (record?.trade_no || '').toLowerCase();
    return Number(record?.amount || 0) === 0 && tradeNo.startsWith('sub');
  };

  const columns = useMemo(() => {
    const baseColumns = [
      ...(userIsAdmin
        ? [
            {
              title: t('用户ID'),
              dataIndex: 'user_id',
              key: 'user_id',
              width: 84,
              render: (userId) => <Text>{userId ?? '-'}</Text>,
            },
          ]
        : []),
      {
        title: t('订单号'),
        dataIndex: 'trade_no',
        key: 'trade_no',
        width: 360,
        render: (text) => (
          <Text
            copyable
            ellipsis={{ showTooltip: true }}
            style={{ maxWidth: 330 }}
          >
            {text}
          </Text>
        ),
      },
      {
        title: t('支付方式'),
        dataIndex: 'payment_method',
        key: 'payment_method',
        width: 112,
        render: renderPaymentMethod,
      },
      {
        title: t('充值额度'),
        dataIndex: 'amount',
        key: 'amount',
        width: 120,
        render: (amount, record) => {
          if (isSubscriptionTopup(record)) {
            return (
              <Tag color='purple' shape='circle' size='small'>
                {t('订阅套餐')}
              </Tag>
            );
          }
          const displayAmount =
            record?.amount_display !== undefined &&
            record?.amount_display !== null
              ? record.amount_display
              : amount;
          return (
            <span className='flex items-center gap-1 whitespace-nowrap'>
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
        width: 112,
        render: (money) => <Text type='danger'>{formatMoney(money)}</Text>,
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        key: 'status',
        width: 96,
        render: renderStatusBadge,
      },
    ];

    baseColumns.push({
      title: t('创建时间'),
      dataIndex: 'create_time',
      key: 'create_time',
      width: 168,
      render: (time) => (
        <Text className='whitespace-nowrap'>{timestamp2string(time)}</Text>
      ),
    });

    baseColumns.push({
      title: t('操作'),
      key: 'action',
      width: 220,
      fixed: 'right',
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
        if (record.status === 'success') {
          actions.push(
            <Button
              key='invoice'
              size='small'
              theme='borderless'
              icon={<FileText size={14} />}
              onClick={() => handleViewInvoice(record)}
            >
              Invoice
            </Button>,
          );
        }
        return (
          <div className='flex items-center gap-2 whitespace-nowrap'>
            {actions}
          </div>
        );
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
        value: detailRecord.create_time
          ? timestamp2string(detailRecord.create_time)
          : '-',
      },
      {
        key: t('完成时间'),
        value: detailRecord.complete_time
          ? timestamp2string(detailRecord.complete_time)
          : '-',
      },
    ];
  }, [detailRecord, t, userIsAdmin]);

  const detailFooter = detailRecord ? (
    <div className='flex items-center justify-end gap-2'>
      {detailRecord.status === 'success' && (
        <>
          <Button
            theme='borderless'
            icon={<FileText size={14} />}
            onClick={() => handleViewInvoice(detailRecord)}
          >
            Invoice
          </Button>
          <Button
            type='primary'
            theme='solid'
            icon={<Download size={14} />}
            onClick={() => handleDownloadInvoice(detailRecord)}
          >
            {t('下载 Invoice')}
          </Button>
        </>
      )}
      <Button onClick={closeDetail}>{t('关闭')}</Button>
    </div>
  ) : null;

  return (
    <>
      <Modal
        title={t('充值账单')}
        visible={visible}
        onCancel={onCancel}
        footer={null}
        size={isMobile ? 'full-width' : 'large'}
        style={isMobile ? undefined : { width: 'min(92vw, 1280px)' }}
        bodyStyle={isMobile ? undefined : { overflow: 'hidden' }}
      >
        <div className='mb-3 grid grid-cols-1 md:grid-cols-2 xl:grid-cols-5 gap-2'>
          <Input
            prefix={<IconSearch />}
            placeholder={t('订单号')}
            value={keyword}
            onChange={handleKeywordChange}
            showClear
          />
          {userIsAdmin && (
            <Input
              prefix={<IconSearch />}
              placeholder={t('用户ID / 用户名 / 邮箱')}
              value={userKeyword}
              onChange={handleUserKeywordChange}
              showClear
            />
          )}
          <div
            className={
              userIsAdmin ? 'xl:col-span-2' : 'md:col-span-1 xl:col-span-2'
            }
          >
            <DatePicker
              className='w-full'
              type='dateTimeRange'
              value={dateRange}
              placeholder={[t('开始时间'), t('结束时间')]}
              onChange={handleDateRangeChange}
              showClear
            />
          </div>
          <Button theme='borderless' onClick={resetFilters}>
            {t('重置')}
          </Button>
        </div>
        <Table
          columns={columns}
          dataSource={topups}
          loading={loading}
          rowKey='id'
          scroll={{ x: userIsAdmin ? 1272 : 1188 }}
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
              image={
                <IllustrationNoResult style={{ width: 150, height: 150 }} />
              }
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
        footer={detailFooter}
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
