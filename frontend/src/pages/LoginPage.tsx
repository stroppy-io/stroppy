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
import { useAuth } from '../contexts/AuthContext';
import { getErrorMessage } from '../services/api';

const { Title, Text } = Typography;

const LoginPage: React.FC = () => {
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  const { login } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();

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
                Вход в систему
              </Title>
              
              <Text type="secondary">
                Добро пожаловать в Stroppy Cloud Panel
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
                label="Имя пользователя"
                rules={[
                  { required: true, message: 'Пожалуйста, введите имя пользователя!' },
                  { min: 3, message: 'Имя пользователя должно содержать минимум 3 символа!' }
                ]}
              >
                <Input
                  prefix={<UserOutlined />}
                  placeholder="Введите имя пользователя"
                  disabled={isLoading}
                />
              </Form.Item>

              <Form.Item
                name="password"
                label="Пароль"
                rules={[
                  { required: true, message: 'Пожалуйста, введите пароль!' },
                  { min: 6, message: 'Пароль должен содержать минимум 6 символов!' }
                ]}
              >
                <Input.Password
                  prefix={<LockOutlined />}
                  placeholder="Введите пароль"
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
                  Войти в систему
                </Button>
              </Form.Item>
            </Form>

            <div style={{ textAlign: 'center', marginTop: '24px' }}>
              <Text type="secondary">
                Нет аккаунта?{' '}
                <Link to="/register" style={{ fontWeight: 500 }}>
                  Создать новый аккаунт
                </Link>
              </Text>
            </div>
          </Card>
        </div>
      </Layout>
  );
};

export default LoginPage;
