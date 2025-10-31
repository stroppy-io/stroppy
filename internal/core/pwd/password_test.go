package pwd

import "testing"

func TestValidatePassword(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "valid password",
			args: args{
				s: "Password123@",
			},
			wantErr: false,
		},
		{
			name: "valid password",
			args: args{
				s: "ForvardShit24422442@",
			},
			wantErr: false,
		},
		{
			name: "valid password 2 ",
			args: args{
				s: "jopaBita/123",
			},
			wantErr: false,
		},
		{
			name: "invalid password",
			args: args{
				s: "pass",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidatePassword(tt.args.s); (err != nil) != tt.wantErr {
				t.Errorf("ValidatePassword() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
