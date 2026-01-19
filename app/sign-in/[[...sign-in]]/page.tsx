import { SignIn } from '@clerk/nextjs';

export default function SignInPage() {
  return (
    <div className="min-h-[80vh] flex items-center justify-center px-6">
      <SignIn
        appearance={{
          elements: {
            rootBox: 'mx-auto',
            card: 'shadow-md border border-border',
            headerTitle: 'text-foreground',
            headerSubtitle: 'text-muted-foreground',
            socialButtonsBlockButton: 'border-border',
            formButtonPrimary: 'bg-primary hover:bg-primary/90',
            footerActionLink: 'text-primary hover:text-primary/80',
            identityPreviewEditButtonIcon: 'text-muted-foreground',
            formFieldInput: 'border-border',
            formFieldLabel: 'text-foreground',
            identityPreviewText: 'text-foreground',
            identityPreviewSecondaryText: 'text-muted-foreground',
          },
        }}
      />
    </div>
  );
}
