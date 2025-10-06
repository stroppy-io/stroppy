import React, { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
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
import { useAuth } from '../contexts/AuthContext';
import { getErrorMessage } from '../services/api';
import { useTranslation } from '../hooks/useTranslation';

const { Title, Text } = Typography;

const RegisterPage: React.FC = () => {
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  const { register } = useAuth();
  const navigate = useNavigate();
  const { t } = useTranslation('common');

  const handleSubmit = async (values: { 
    username: string; 
    password: string; 
    confirmPassword: string; 
  }) => {
    setError('');

    if (values.password !== values.confirmPassword) {
      setError(t('auth.register.passwordsNotMatchError'));
      return;
    }

    try {
      setIsLoading(true);
      await register({
        username: values.username.trim(),
        password: values.password,
      });
      navigate('/');
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
                {t('auth.register.title')}
              </Title>
              
              <Text type="secondary">
                {t('auth.register.subtitle')}
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
              name="register"
              onFinish={handleSubmit}
              layout="vertical"
              size="large"
            >
              <Form.Item
                name="username"
                label={t('auth.register.username')}
                rules={[
                  { required: true, message: t('auth.register.usernameRequired') },
                  { min: 3, message: t('auth.register.usernameMinLength') },
                  { max: 50, message: t('auth.register.usernameMaxLength') },
                  { 
                    pattern: /^[a-zA-Z0-9_-]+$/, 
                    message: t('auth.register.usernamePattern') 
                  }
                ]}
              >
                <Input
                  prefix={<UserOutlined />}
                  placeholder={t('auth.register.usernamePlaceholder')}
                  disabled={isLoading}
                />
              </Form.Item>

              <Form.Item
                name="password"
                label={t('auth.register.password')}
                rules={[
                  { required: true, message: t('auth.register.passwordRequired') },
                  { min: 6, message: t('auth.register.passwordMinLength') }
                ]}
              >
                <Input.Password
                  prefix={<LockOutlined />}
                  placeholder={t('auth.register.passwordPlaceholder')}
                  disabled={isLoading}
                />
              </Form.Item>

              <Form.Item
                name="confirmPassword"
                label={t('auth.register.confirmPassword')}
                dependencies={['password']}
                rules={[
                  { required: true, message: t('auth.register.confirmPasswordRequired') },
                  ({ getFieldValue }) => ({
                    validator(_, value) {
                      if (!value || getFieldValue('password') === value) {
                        return Promise.resolve();
                      }
                      return Promise.reject(new Error(t('auth.register.passwordsNotMatch')));
                    },
                  }),
                ]}
              >
                <Input.Password
                  prefix={<LockOutlined />}
                  placeholder={t('auth.register.confirmPasswordPlaceholder')}
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
                  {t('auth.register.submitButton')}
                </Button>
              </Form.Item>
            </Form>

            <div style={{ textAlign: 'center', marginTop: '24px' }}>
              <Text type="secondary">
                {t('auth.register.hasAccount')}{' '}
                <Link to="/login" style={{ fontWeight: 500 }}>
                  {t('auth.register.loginLink')}
                </Link>
              </Text>
            </div>
          </Card>
        </div>
      </Layout>
  );
};

export default RegisterPage;