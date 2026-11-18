package access

const (
	OpMFAChallenge              Operation = "MFAChallenge"
	OpMFAVerify                 Operation = "MFAVerify"
	OpMFALoginEnrollmentStart   Operation = "MFALoginEnrollmentStart"
	OpMFALoginEnrollmentConfirm Operation = "MFALoginEnrollmentConfirm"
	OpMFARecoveryClaim          Operation = "MFARecoveryClaim"
	OpMFACancel                 Operation = "MFACancel"
	OpMFARead                   Operation = "MFARead"
	OpMFAEnrollmentStart        Operation = "MFAEnrollmentStart"
	OpMFAEnrollmentConfirm      Operation = "MFAEnrollmentConfirm"
	OpMFAEnrollmentCancel       Operation = "MFAEnrollmentCancel"
	OpMFADisable                Operation = "MFADisable"
	OpMFARecoveryRegenerate     Operation = "MFARecoveryRegenerate"
	OpSecurityPolicyRead        Operation = "SecurityPolicyRead"
	OpSecurityPolicyImpact      Operation = "SecurityPolicyImpact"
	OpSecurityPolicyUpdate      Operation = "SecurityPolicyUpdate"
	OpAccountMFARead            Operation = "AccountMFARead"
	OpAccountMFAReset           Operation = "AccountMFAReset"
)
