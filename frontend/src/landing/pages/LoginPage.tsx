import React, { useState } from 'react';
import { Link, useNavigate, useLocation } from 'react-router-dom';
import { 
  Card, 
  Form, 
  Input, 
  Button, 
  Typography, 
  Alert, 
  Layout
} from 'antd';
import { 
  UserOutlined, 
  LockOutlined, 
  CloudOutlined
} from '@ant-design/icons';
import { useAuth } from '../../common/contexts/AuthContext';
import { getErrorMessage } from '../../common/services/api';
import { useTranslation } from '../../common/hooks/useTranslation';

const { Title, Text } = Typography;

const LoginPage: React.FC = () => {
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  const { login } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const { t } = useTranslation('common');

  // Получаем путь, откуда пользователь был перенаправлен
  const from = location.state?.from?.pathname || '/';

  const handleSubmit = async (values: { username: string; password: string }) => {
    setError('');

    try {
      setIsLoading(true);
      await login({ username: values.username.trim(), password: values.password });
      navigate(from, { replace: true });
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <Layout style={{ minHeight: '100vh', background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)' }}>
        <div style={{
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          minHeight: '100vh',
          padding: '24px',
          position: 'relative'
        }}>

          <Card
            style={{
              width: '100%',
              maxWidth: '400px',
              boxShadow: '0 8px 32px rgba(0,0,0,0.12)'
            }}
          >
            <div style={{ textAlign: 'center', marginBottom: '32px' }}>
              <div style={{
                width: '64px',
                height: '64px',
                background: 'linear-gradient(135deg, #1890ff, #096dd9)',
                borderRadius: '16px',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                margin: '0 auto 16px'
              }}>
                <CloudOutlined style={{ color: 'white', fontSize: '32px' }} />
              </div>
              
              <Title level={2} style={{ margin: '0 0 8px 0' }}>
                {t('auth.login.title')}
              </Title>
              
              <Text type="secondary">
                {t('auth.login.subtitle')}
              </Text>
            </div>

            {error && (
              <Alert
                message={error}
                type="error"
                showIcon
                style={{ marginBottom: '24px' }}
              />
            )}

            <Form
              name="login"
              onFinish={handleSubmit}
              layout="vertical"
              size="large"
            >
              <Form.Item
                name="username"
                label={t('auth.login.username')}
                rules={[
                  { required: true, message: t('auth.login.usernameRequired') },
                  { min: 3, message: t('auth.login.usernameMinLength') }
                ]}
              >
                <Input
                  prefix={<UserOutlined />}
                  placeholder={t('auth.login.usernamePlaceholder')}
                  disabled={isLoading}
                />
              </Form.Item>

              <Form.Item
                name="password"
                label={t('auth.login.password')}
                rules={[
                  { required: true, message: t('auth.login.passwordRequired') },
                  { min: 6, message: t('auth.login.passwordMinLength') }
                ]}
              >
                <Input.Password
                  prefix={<LockOutlined />}
                  placeholder={t('auth.login.passwordPlaceholder')}
                  disabled={isLoading}
                />
              </Form.Item>

              <Form.Item style={{ marginBottom: '16px' }}>
                <Button
                  type="primary"
                  htmlType="submit"
                  loading={isLoading}
                  block
                  style={{ height: '44px', fontSize: '16px' }}
                >
                  {t('auth.login.submitButton')}
                </Button>
              </Form.Item>
            </Form>

            <div style={{ textAlign: 'center', marginTop: '24px' }}>
              <Text type="secondary">
                {t('auth.login.noAccount')}{' '}
                <Link to="/register" style={{ fontWeight: 500 }}>
                  {t('auth.login.createAccount')}
                </Link>
              </Text>
            </div>
          </Card>
        </div>
      </Layout>
  );
};

export default LoginPage;
