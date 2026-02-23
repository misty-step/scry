import type { ReactElement } from 'react';
import { render } from '@testing-library/react';
import { vi } from 'vitest';
import {
  ReviewSessionContext,
  type ReviewSessionContextValue,
} from '@/components/review/session-context';

type BaseQuestion = NonNullable<ReviewSessionContextValue['question']>;
type BaseDisplayQuestion = NonNullable<ReviewSessionContextValue['displayQuestion']>;

type SessionOverrides = Partial<
  Omit<ReviewSessionContextValue, 'feedbackState' | 'unifiedEdit' | 'question' | 'displayQuestion'>
> & {
  feedbackState?: Partial<ReviewSessionContextValue['feedbackState']>;
  unifiedEdit?: Partial<ReviewSessionContextValue['unifiedEdit']> & {
    localData?: Partial<ReviewSessionContextValue['unifiedEdit']['localData']>;
    errors?: Record<string, string>;
  };
  question?: Partial<BaseQuestion> | null;
  displayQuestion?: Partial<BaseDisplayQuestion> | null;
};

const baseQuestion: BaseQuestion = {
  question: 'What is 2+2?',
  correctAnswer: '4',
  explanation: 'Basic arithmetic',
  options: ['2', '3', '4', '5'],
  type: 'multiple-choice',
};

export function createMockSession(overrides: SessionOverrides = {}): ReviewSessionContextValue {
  const baseSession: ReviewSessionContextValue = {
    phase: 'reviewing',
    question: baseQuestion,
    conceptTitle: 'Math Basics',
    conceptId: 'concept123' as ReviewSessionContextValue['conceptId'],
    phrasingId: 'phrasing123' as ReviewSessionContextValue['phrasingId'],
    phrasingIndex: 1,
    totalPhrasings: 3,
    selectionReason: 'random',
    interactions: [],
    isTransitioning: false,
    conceptFsrs: null,
    selectedAnswer: '',
    feedbackState: { showFeedback: false, nextReviewInfo: null },
    hasAnsweredCurrentQuestion: false,
    currentInteractionId: null,
    userFeedback: null,
    displayConceptTitle: 'Math Basics',
    displayQuestion: baseQuestion,
    phrasingPositionLabel: 'Phrasing 1 of 3',
    selectionReasonLabel: 'Random rotation',
    instantFeedback: { visible: false, isCorrect: false },
    skipAnnouncement: '',
    cachedDueCount: 5,
    unifiedEdit: {
      isEditing: false,
      isSaving: false,
      isDirty: false,
      conceptIsDirty: false,
      phrasingIsDirty: false,
      localData: {
        conceptTitle: 'Math Basics',
        question: 'What is 2+2?',
        correctAnswer: '4',
        explanation: 'Basic arithmetic',
        options: ['2', '3', '4', '5'],
      },
      errors: {},
      startEdit: vi.fn(),
      save: vi.fn().mockResolvedValue(undefined),
      cancel: vi.fn(),
      updateField: vi.fn(),
    },
    handleAnswerSelect: vi.fn(),
    handleSubmit: vi.fn(),
    handleNext: vi.fn(),
    handleSkip: vi.fn(),
    handleArchivePhrasing: vi.fn().mockResolvedValue(undefined),
    handleArchiveConcept: vi.fn().mockResolvedValue(undefined),
    handleUserFeedback: vi.fn().mockResolvedValue(undefined),
    handleStartInlineEdit: vi.fn(),
    handleArchiveViaShortcut: vi.fn(),
    handleQuestionDisplayReset: vi.fn(),
  };

  const mergedQuestion =
    overrides.question === null
      ? null
      : ({ ...baseSession.question, ...(overrides.question ?? {}) } as BaseQuestion);

  const mergedDisplayQuestion =
    overrides.displayQuestion === null
      ? null
      : ({
          ...(baseSession.displayQuestion ?? baseQuestion),
          ...(overrides.displayQuestion ?? {}),
        } as BaseDisplayQuestion);

  return {
    ...baseSession,
    ...overrides,
    question: mergedQuestion,
    displayQuestion: mergedDisplayQuestion,
    feedbackState: {
      ...baseSession.feedbackState,
      ...(overrides.feedbackState ?? {}),
    },
    unifiedEdit: {
      ...baseSession.unifiedEdit,
      ...(overrides.unifiedEdit ?? {}),
      localData: {
        ...baseSession.unifiedEdit.localData,
        ...(overrides.unifiedEdit?.localData ?? {}),
      },
      errors: {
        ...baseSession.unifiedEdit.errors,
        ...(overrides.unifiedEdit?.errors ?? {}),
      },
    },
  };
}

export function renderWithSession(ui: ReactElement, sessionOverrides: SessionOverrides = {}) {
  const session = createMockSession(sessionOverrides);
  return {
    session,
    ...render(<ReviewSessionContext.Provider value={session}>{ui}</ReviewSessionContext.Provider>),
  };
}
