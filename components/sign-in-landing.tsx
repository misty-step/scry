'use client';

import { SignIn } from '@clerk/nextjs';
import { ArrowDown, Brain, Clock, Keyboard, Sparkles, Zap } from 'lucide-react';
import { Button } from '@/components/ui/button';

/**
 * Landing page for unauthenticated users
 *
 * Structure:
 * 1. Hero - Logo, value prop, CTA
 * 2. Features - 4 key differentiators
 * 3. Philosophy - "Brutal honesty" positioning
 * 4. Auth - Clerk sign-in
 */
export function SignInLanding() {
  const scrollToAuth = () => {
    document.getElementById('auth-section')?.scrollIntoView({ behavior: 'smooth' });
  };

  return (
    <div className="min-h-screen">
      {/* Hero Section */}
      <section className="min-h-[90vh] flex items-center justify-center px-6 py-12">
        <div className="w-full max-w-6xl">
          <div className="grid lg:grid-cols-2 gap-12 lg:gap-16 items-center">
            {/* Left: Branding + Value Prop */}
            <div className="space-y-8">
              <div>
                <h1 className="text-6xl md:text-7xl lg:text-8xl font-bold tracking-tight text-foreground">
                  Scry<span className="opacity-70">.</span>
                </h1>
                <p className="text-2xl md:text-3xl text-muted-foreground mt-4">
                  Remember everything.
                </p>
              </div>

              <p className="text-lg md:text-xl text-foreground/80 max-w-lg leading-relaxed">
                AI-powered quiz generation meets scientifically optimal spaced repetition. Generate
                questions on any topic. Review at the perfect time.{' '}
                <span className="font-medium">No excuses.</span>
              </p>

              <div className="flex flex-col sm:flex-row gap-4">
                <Button size="lg" onClick={scrollToAuth} className="text-base">
                  Get Started Free
                  <ArrowDown className="ml-2 h-4 w-4" />
                </Button>
                <Button size="lg" variant="outline" onClick={scrollToAuth} className="text-base">
                  Sign In
                </Button>
              </div>
            </div>

            {/* Right: Feature Preview */}
            <div className="hidden lg:block">
              <div className="relative">
                {/* Mock quiz card */}
                <div className="bg-card border rounded-xl p-6 shadow-lg">
                  <div className="space-y-4">
                    <div className="flex items-center gap-2 text-sm text-muted-foreground">
                      <Clock className="h-4 w-4" />
                      <span>12 concepts due</span>
                    </div>
                    <p className="text-lg font-medium">
                      What is the primary function of mitochondria in eukaryotic cells?
                    </p>
                    <div className="space-y-2">
                      {[
                        'Energy production (ATP)',
                        'Protein synthesis',
                        'Cell division',
                        'Waste removal',
                      ].map((option, i) => (
                        <div
                          key={option}
                          className={`p-3 rounded-lg border transition-colors ${
                            i === 0
                              ? 'border-primary bg-primary/5'
                              : 'border-border hover:border-border/80'
                          }`}
                        >
                          <span className="text-sm font-medium mr-2 text-muted-foreground">
                            {i + 1}
                          </span>
                          {option}
                        </div>
                      ))}
                    </div>
                  </div>
                </div>
                {/* Decorative elements */}
                <div className="absolute -top-4 -right-4 w-24 h-24 bg-primary/10 rounded-full blur-2xl" />
                <div className="absolute -bottom-4 -left-4 w-32 h-32 bg-primary/5 rounded-full blur-3xl" />
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Features Section */}
      <section className="py-20 px-6 bg-muted/30">
        <div className="w-full max-w-6xl mx-auto">
          <div className="text-center mb-12">
            <h2 className="text-3xl md:text-4xl font-bold tracking-tight">
              Not another Anki clone
            </h2>
            <p className="text-lg text-muted-foreground mt-4 max-w-2xl mx-auto">
              Built for serious learners who want results, not comfort features.
            </p>
          </div>

          <div className="grid md:grid-cols-2 lg:grid-cols-4 gap-6">
            <FeatureCard
              icon={<Sparkles className="h-6 w-6" />}
              title="AI Generation"
              description="Generate quiz questions on any topic in seconds. No manual card creation."
            />
            <FeatureCard
              icon={<Brain className="h-6 w-6" />}
              title="Pure FSRS"
              description="State-of-the-art spaced repetition. No tweaks, no shortcuts, just science."
            />
            <FeatureCard
              icon={<Zap className="h-6 w-6" />}
              title="No Daily Limits"
              description="300 cards due? Review 300 cards. The algorithm doesn't negotiate."
            />
            <FeatureCard
              icon={<Keyboard className="h-6 w-6" />}
              title="Keyboard-First"
              description="Power through reviews with shortcuts. 1-4 to answer, Enter to continue."
            />
          </div>
        </div>
      </section>

      {/* Philosophy Section */}
      <section className="py-20 px-6">
        <div className="w-full max-w-4xl mx-auto text-center">
          <h2 className="text-3xl md:text-4xl font-bold tracking-tight mb-6">
            Brutal honesty about learning
          </h2>
          <div className="space-y-6 text-lg text-foreground/80">
            <p>
              Most spaced repetition apps let you skip cards, set daily limits, and pretend
              you&apos;re learning. We don&apos;t.
            </p>
            <p>
              The forgetting curve doesn&apos;t care about your comfort. Generate 50 questions?
              Review 50 questions. The natural consequences teach sustainable habits.
            </p>
            <p className="text-xl font-medium text-foreground">
              Every &quot;enhancement&quot; that makes spaced repetition more comfortable makes it
              less effective.
            </p>
          </div>
        </div>
      </section>

      {/* Auth Section */}
      <section id="auth-section" className="py-20 px-6 bg-muted/30">
        <div className="w-full max-w-md mx-auto">
          <div className="text-center mb-8">
            <h2 className="text-2xl font-bold tracking-tight">Start learning</h2>
            <p className="text-muted-foreground mt-2">Free to use. No credit card required.</p>
          </div>
          <SignIn
            routing="hash"
            appearance={{
              elements: {
                rootBox: 'mx-0 w-full',
                card: 'shadow-none border p-6 bg-card rounded-xl w-full',
                headerTitle: 'hidden',
                headerSubtitle: 'hidden',
                socialButtonsBlockButton: 'border-border',
                dividerRow: 'hidden',
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
      </section>
    </div>
  );
}

function FeatureCard({
  icon,
  title,
  description,
}: {
  icon: React.ReactNode;
  title: string;
  description: string;
}) {
  return (
    <div className="bg-card border rounded-xl p-6 space-y-3">
      <div className="text-primary">{icon}</div>
      <h3 className="font-semibold text-lg">{title}</h3>
      <p className="text-sm text-muted-foreground leading-relaxed">{description}</p>
    </div>
  );
}
