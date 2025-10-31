import React from 'react';
import { Button, Dropdown, Switch } from 'antd';
import { 
  SettingOutlined, 
  SunOutlined, 
  MoonOutlined, 
  GlobalOutlined,
  CheckOutlined
} from '@ant-design/icons';
import { useTheme } from '../../common/contexts/ThemeContext';
import { useTranslation } from '../../common/hooks/useTranslation';

const SettingsDropdown: React.FC = () => {
  const { t } = useTranslation('landing');
  const { changeLanguage, getCurrentLanguage, getAvailableLanguages } = useTranslation();
  const { darkReaderEnabled, toggleDarkReader } = useTheme();

  const currentLanguage = getCurrentLanguage();
  const availableLanguages = getAvailableLanguages();

  const handleLanguageChange = (languageCode: string) => {
    changeLanguage(languageCode);
  };

  const menuItems = [
    {
      key: 'theme',
      label: (
        <div style={{ padding: '4px 0' }}>
          <div style={{ fontWeight: 'bold', marginBottom: '8px' }}>
            {t('settings.theme.dark')}
          </div>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <span>{darkReaderEnabled ? t('settings.theme.dark') : t('settings.theme.light')}</span>
            <Switch
              checked={darkReaderEnabled}
              onChange={toggleDarkReader}
              checkedChildren={<MoonOutlined />}
              unCheckedChildren={<SunOutlined />}
              size="small"
            />
          </div>
        </div>
      ),
      disabled: true
    },
    {
      type: 'divider' as const
    },
    {
      key: 'language',
      label: (
        <div style={{ padding: '4px 0' }}>
          <div style={{ fontWeight: 'bold', marginBottom: '8px' }}>
            <GlobalOutlined style={{ marginRight: '8px' }} />
            Язык / Language
          </div>
          {availableLanguages.map((lang) => (
            <div
              key={lang.code}
              onClick={() => handleLanguageChange(lang.code)}
              style={{
                padding: '4px 8px',
                cursor: 'pointer',
                borderRadius: '4px',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                backgroundColor: currentLanguage === lang.code ? 'rgba(24, 144, 255, 0.1)' : 'transparent',
                marginBottom: '4px'
              }}
            >
              <span>{lang.nativeName}</span>
              {currentLanguage === lang.code && (
                <CheckOutlined style={{ color: '#1890ff' }} />
              )}
            </div>
          ))}
        </div>
      ),
      disabled: true
    }
  ];

  return (
    <Dropdown
      menu={{ items: menuItems }}
      placement="bottomRight"
      trigger={['click']}
    >
      <Button 
        type="text" 
        icon={<SettingOutlined />}
        style={{ 
          color: '#666',
          display: 'flex',
          alignItems: 'center',
          gap: '4px'
        }}
      />
    </Dropdown>
  );
};

export default SettingsDropdown;
